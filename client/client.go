package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"time"
)

type OpsGenieClient struct {
	RetryableClient *retryablehttp.Client
	Config          Config
}

type request struct {
	*retryablehttp.Request
}

type ApiRequest interface {
	Validate() (bool, error)
	Endpoint() string
	Method() string
}

type apiResult interface {
	setRequestID(requestId string)
	setResponseTime(responseTime float32)
	setRateLimitState(state string)
}

type ResponseMeta struct {
	RequestID      string
	ResponseTime   float32
	RateLimitState string
}

func (rm *ResponseMeta) setRequestID(requestID string) {
	rm.RequestID = requestID
}

func (rm *ResponseMeta) setResponseTime(responseTime float32) {
	rm.ResponseTime = responseTime
}

func (rm *ResponseMeta) setRateLimitState(state string) {
	rm.RateLimitState = state
}

var apiURL = "https://api.opsgenie.com"
var euApiURL = "https://api.eu.opsgenie.com"
var UserAgentHeader string

func setConfiguration(opsGenieClient *OpsGenieClient, cfg Config) {
	opsGenieClient.RetryableClient.ErrorHandler = defineErrorHandler
	opsGenieClient.Config.apiUrl = apiURL
	if cfg.OpsGenieAPIURL == API_URL_EU {
		opsGenieClient.Config.apiUrl = euApiURL
	}
	if cfg.HttpClient != nil {
		opsGenieClient.RetryableClient.HTTPClient = cfg.HttpClient
	}
}

func setLogger(conf *Config) {
	logrus.SetFormatter(
		&logrus.TextFormatter{
			ForceColors:     true,
			FullTimestamp:   true,
			TimestampFormat: time.RFC3339Nano,
		},
	)
	logrus.SetLevel(logrus.InfoLevel)
	if conf.LogLevel != "" {
		lvl, err := logrus.ParseLevel(conf.LogLevel)
		if err != nil {
			logrus.Infof("Error occurred: %s. Setting log level as info", err.Error())
		} else {
			logrus.SetLevel(lvl)
		}
	}
	conf.LogLevel = logrus.GetLevel().String()
}

func setProxy(client *OpsGenieClient, proxyUrl string) error {
	if proxyUrl != "" {
		proxyURL, err := url.Parse(proxyUrl)
		if err != nil {
			return err
		}
		client.RetryableClient.HTTPClient.Transport = &http.Transport{Proxy: http.ProxyURL(proxyURL)}
	}
	return nil
}

func setRetryPolicy(opsGenieClient *OpsGenieClient, cfg Config) {
	//custom backoff
	if cfg.Backoff != nil {
		opsGenieClient.RetryableClient.Backoff = cfg.Backoff
	}

	//custom retry policy
	if cfg.RetryPolicy != nil { //todo:429 retry etmeli
		opsGenieClient.RetryableClient.CheckRetry = cfg.RetryPolicy
	} else {
		opsGenieClient.RetryableClient.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (b bool, e error) {
			if ctx.Err() != nil {
				return false, ctx.Err()
			}

			if err != nil {
				return true, err
			}
			// Check the response code. We retry on 500-range responses to allow
			// the server time to recover, as 500's are typically not permanent
			// errors and may relate to outages on the server side. This will catch
			// invalid response codes as well, like 0 and 999.
			if resp.StatusCode == 0 || (resp.StatusCode >= 500 && resp.StatusCode != 501) {
				return true, nil
			}
			if resp.StatusCode == 429 {
				return true, nil
			}

			return false, nil
		}
	}

	if cfg.RetryCount != 0 {
		opsGenieClient.RetryableClient.RetryMax = cfg.RetryCount
	} else {
		opsGenieClient.RetryableClient.RetryMax = 4
	}
}

func NewOpsGenieClient(cfg Config) (*OpsGenieClient, error) {
	UserAgentHeader = fmt.Sprintf("%s %s (%s/%s)", "opsgenie-go-sdk-v2", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	opsGenieClient := &OpsGenieClient{
		Config:          cfg,
		RetryableClient: retryablehttp.NewClient(),
	}
	if cfg.ApiKey == "" {
		return nil, errors.New("API key cannot be blank")
	}
	setConfiguration(opsGenieClient, cfg)
	opsGenieClient.RetryableClient.Logger = nil //disable retryableClient's uncustomizable logging
	setLogger(&cfg)
	err := setProxy(opsGenieClient, cfg.ProxyUrl)
	if err != nil {
		return nil, err
	}
	setRetryPolicy(opsGenieClient, cfg)
	if err != nil {
		return nil, err
	}
	printInfoLog(opsGenieClient)
	return opsGenieClient, nil
}

func printInfoLog(client *OpsGenieClient) {
	logrus.Infof("Client is configured with ApiKey: %s, ApiUrl: %s, ProxyUrl: %s, LogLevel: %s, RetryMaxCount: %v",
		client.Config.ApiKey,
		client.Config.apiUrl,
		client.Config.ProxyUrl,
		logrus.GetLevel().String(),
		client.RetryableClient.RetryMax)
}

func defineErrorHandler(resp *http.Response, err error, numTries int) (*http.Response, error) {
	if err != nil {
		logrus.Errorf("Unable to send the request %s ", err.Error())
		if err == context.DeadlineExceeded {
			return nil, err
		}
		return nil, err
	}
	logrus.Errorf("Failed to process request after %d retries.", numTries)
	return resp, nil
}

func (cli *OpsGenieClient) do(request *request) (*http.Response, error) {
	return cli.RetryableClient.Do(request.Request)
}

func setResponseMeta(httpResponse *http.Response, result apiResult) {
	requestID := httpResponse.Header.Get("X-Request-Id")
	result.setRequestID(requestID)

	rateLimitState := httpResponse.Header.Get("X-RateLimit-State")
	result.setRateLimitState(rateLimitState)

	responseTime, err := strconv.ParseFloat(httpResponse.Header.Get("X-Response-Time"), 32)
	if err == nil {
		result.setResponseTime(float32(responseTime))
	}

}

type ApiError struct {
	error
	Message     string            `json:"message"`
	Took        float32           `json:"took"`
	RequestId   string            `json:"requestId"`
	Errors      map[string]string `json:"errors"`
	StatusCode  string
	ErrorHeader string
}

func (ar ApiError) Error() string {
	errMessage := "Error occurred with Status code: " + ar.StatusCode + ", " +
		"Message: " + ar.Message + ", " +
		"Took: " + fmt.Sprintf("%f", ar.Took) + ", " +
		"RequestId: " + ar.RequestId
	if ar.ErrorHeader != "" {
		errMessage = errMessage + ", Error Header: " + ar.ErrorHeader
	}
	if ar.Errors != nil {
		errMessage = errMessage + ", Error Detail: " + fmt.Sprintf("%v", ar.Errors)
	}
	return errMessage
}

func handleErrorIfExist(response *http.Response) error {
	if response != nil && response.StatusCode >= 300 {
		apiError := &ApiError{}
		statusCode := response.StatusCode
		apiError.StatusCode = strconv.Itoa(statusCode)
		apiError.ErrorHeader = response.Header.Get("X-Opsgenie-Errortype")
		body, _ := ioutil.ReadAll(response.Body)
		json.Unmarshal(body, apiError)
		return apiError
	}
	return nil
}

func (cli *OpsGenieClient) buildHttpRequest(apiRequest ApiRequest) (*request, error) {
	var buf io.ReadWriter
	if apiRequest.Method() != "GET" && apiRequest.Method() != "DELETE" {
		buf = new(bytes.Buffer)
		err := json.NewEncoder(buf).Encode(apiRequest)
		if err != nil {
			return nil, err
		}
	}

	req, err := retryablehttp.NewRequest(apiRequest.Method(), cli.Config.apiUrl+apiRequest.Endpoint(), buf)
	if err != nil {
		return nil, err
	}

	if apiRequest.Method() != "GET" && apiRequest.Method() != "DELETE" {
		req.Header.Add("Content-Type", "application/json; charset=utf-8")
	}
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", "GenieKey "+cli.Config.ApiKey)
	req.Header.Add("User-Agent", UserAgentHeader)
	if apiRequest.Method() == "GET" {
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	}
	return &request{req}, err

}

func (cli *OpsGenieClient) Exec(ctx context.Context, request ApiRequest, result apiResult) error {

	logrus.Debugf("Starting to process Request %+v: to send: %s", request, request.Endpoint())
	if ok, err := request.Validate(); !ok {
		logrus.Errorf("Request validation err: %s ", err.Error())
		return err
	}
	req, err := cli.buildHttpRequest(request)
	if err != nil {
		logrus.Errorf("Could not create request: %s", err.Error())
		return err
	}
	if ctx != nil {
		req.WithContext(ctx)
	}

	response, err := cli.do(req)
	if err != nil {
		logrus.Errorf(err.Error())
		return err
	}

	defer response.Body.Close()

	err = handleErrorIfExist(response)
	if err != nil {
		logrus.Errorf(err.Error())
		return err
	}

	err = parse(response, result)
	if err != nil {
		logrus.Errorf(err.Error())
		return err
	}

	logrus.Debugf("Request processed. The result: %+v", result)
	return err
}

func parse(response *http.Response, result apiResult) error {
	if response == nil {
		return errors.New("No response received")
	}
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(body, result)
	if err != nil {
		message := "Response could not be parsed, " + err.Error()
		return errors.New(message)

	}
	setResponseMeta(response, result)

	return nil
}
