package main

import (
	"context"
	"fmt"
	"opsgenie-go-sdk-v2/client"
	"opsgenie-go-sdk-v2/heartbeat"
	"time"
)

func main() {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 1*time.Nanosecond)
	defer cancel()

	/*heartbeatClient := heartbeat.NewClient(client.Config{
		ApiKey:         "a871eb83-2d00-4b09-9fb9-7c134a369082",
		OpsGenieAPIURL: "https://api.opsgenie.com",
	})*/

	conf := client.Config{
		ApiKey:         "a871eb83-2d00-4b09-9fb9-7c134a369082",
		OpsGenieAPIURL: client.API_URL_EU,
	}
	conf.LogLevel = "iasdasd"
	conf.RetryCount = 1
	//conf.ProxyUrl = "httpadss://faceasdbook.com"
	heartbeatClient := heartbeat.NewClient(conf)

	/*pr := heartbeat.PingRequest{}

	res, err := heartbeatClient.Ping(pr)

	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(res)
	}*/

	gr := heartbeat.GetRequest{HeartbeatName: "NewSDK"}

	res, err := heartbeatClient.Get(gr, nil)

	if err != nil {
		fmt.Println(err.Error())
	}
	fmt.Printf("%+v", res)

	/*res, err = heartbeatClient.List()

	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(res)

	updateHeartbeatRequest := heartbeat.UpdateRequest{Name: "NewSDK", Description: "Descriptionnn", Interval: 2, IntervalUnit: heartbeat.Minutes, Enabled: true, OwnerTeam: heartbeat.OwnerTeam{Name: "Sales"}}
	res, err := heartbeatClient.Update(updateHeartbeatRequest)

	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(res)

	addHeartbeatRequest := heartbeat.AddRequest{Name: "NewDesign", Description: "Description", Interval: 22, IntervalUnit: heartbeat.Minutes, Enabled: true, OwnerTeam: heartbeat.OwnerTeam{Name: "Sales"}}
	res, err := heartbeatClient.Add(addHeartbeatRequest)

	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(res)

	res, err := heartbeatClient.Enable("NewDesign")

	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(res)

	res, err := heartbeatClient.Disable("NewDesign")

	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(res)*/

}
