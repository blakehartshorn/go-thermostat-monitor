package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
)

// Need globally defined
var NestAccessToken string

// RefreshLogin - Routinely fetch a new authentication token
func NestRefreshLogin(config ConfigRoot) {

	// Authorization - unpack access_token
	type Authorization struct {
		AccessToken string `json:"access_token"`
	}

	httpClient := &http.Client{Timeout: time.Second * 10}
	postData, _ := json.Marshal(map[string]string{
		"client_id":     config.NestConfig.ClientID,
		"client_secret": config.NestConfig.ClientSecret,
		"grant_type":    "refresh_token",
		"redirect_uri":  config.NestConfig.RedirectUri,
		"refresh_token": config.NestConfig.RefreshToken,
	})
	for {
		log.Println("Getting new Google access_token")
		res, err := httpClient.Post(
			"https://www.googleapis.com/oauth2/v4/token",
			"application/json",
			bytes.NewBuffer(postData),
		)
		if err != nil {
			log.Println("ERROR: Could not login to Google.")
			log.Fatal(err)
		}
		var authData Authorization
		body, _ := ioutil.ReadAll(res.Body)
		err = json.Unmarshal(body, &authData)
		res.Body.Close()
		if err != nil {
			log.Println("ERROR: Invalid response object from Google")
			log.Fatal(err)
		}
		NestAccessToken = fmt.Sprintf("Bearer %s", authData.AccessToken)
		time.Sleep(time.Minute * 45)
	}
}

// WriteNest - parse and write thermostat data to influx
func WriteNest(
	config ConfigRoot,
	influxClient influxdb2.Client) {

	// Traits - traits per device json object
	type Traits struct {
		Info struct {
			CustomName string `json:"customName"`
		} `json:"sdm.devices.traits.Info"`
		Humidity struct {
			Percent int `json:"ambientHumidityPercent"`
		} `json:"sdm.devices.traits.Humidity"`
		Connectivity struct {
			Status string `json:"status"`
		} `json:"sdm.devices.traits.Connectivity"`
		ThermostatMode struct {
			Mode string `json:"mode"`
		} `json:"sdm.devices.traits.ThermostatMode"`
		ThermostatEco struct {
			Mode string  `json:"mode"`
			Heat float64 `json:"heatCelsius"`
			Cool float64 `json:"coolCelsius"`
		} `json:"sdm.devices.traits.ThermostatEco"`
		ThermostatHvac struct {
			Status string `json:"status"`
		} `json:"sdm.devices.traits.ThermostatHvac"`
		ThermostatTemperatureSetpoint struct {
			Heat float64 `json:"heatCelsius"`
			Cool float64 `json:"coolCelsius"`
		} `json:"sdm.devices.traits.ThermostatTemperatureSetpoint"`
		Temperature struct {
			Ambient float64 `json:"ambientTemperatureCelsius"`
		} `json:"sdm.devices.traits.Temperature"`
	}

	// Device - Individual devices and their descriptions
	type Device struct {
		Name            string `json:"name"`
		Type            string `json:"type"`
		Assignee        string `json:"assignee"`
		Traits          Traits `json:"traits"`
		ParentRelations []struct {
			DisplayName string `json:"displayName"`
			Parent      string `json:"parent"`
		} `json:"parentRelations"`
	}

	// Devices - root of the device list response
	type Devices struct {
		Device []Device `json:"devices"`
	}

	writer := influxClient.WriteAPIBlocking(config.InfluxConfig.Org, config.InfluxConfig.Bucket)

	url := fmt.Sprintf(
		"https://smartdevicemanagement.googleapis.com/v1/enterprises/%s/devices",
		config.NestConfig.ProjectID,
	)

	for {

		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", NestAccessToken)

		httpClient := &http.Client{Timeout: time.Second * 10}
		res, err := httpClient.Do(req)
		if err != nil {
			log.Println("ERROR: Could not get device info from Nest API.")
			log.Fatal(err)
		}

		var NestDevices Devices
		body, _ := ioutil.ReadAll(res.Body)
		err = json.Unmarshal(body, &NestDevices)
		res.Body.Close()
		if err != nil {
			log.Print(string(body))
			log.Println("ERROR: Invalid json.")
			log.Fatal(err)
		}

		for _, device := range NestDevices.Device {

			var Tags = make(map[string]string)
			var Fields = make(map[string]interface{})

			Tags["name"] = device.Name
			Tags["assignee"] = device.Assignee
			Tags["customName"] = device.Traits.Info.CustomName

			for _, parent := range device.ParentRelations {
				if device.Assignee == parent.Parent {
					Tags["displayName"] = parent.DisplayName
					break
				}
			}

			if device.Traits.Connectivity.Status == "ONLINE" && device.Type == "sdm.devices.types.THERMOSTAT" {
				Fields["humidity"] = device.Traits.Humidity.Percent
				Fields["temperature"] = device.Traits.Temperature.Ambient
				if device.Traits.ThermostatEco.Mode == "MANUAL_ECO" {
					Fields["heat"] = device.Traits.ThermostatEco.Heat
					Fields["cool"] = device.Traits.ThermostatEco.Cool
					Tags["mode"] = "MANUAL_ECO"
				} else if device.Traits.ThermostatMode.Mode == "HEATCOOL" {
					Fields["heat"] = device.Traits.ThermostatTemperatureSetpoint.Heat
					Fields["cool"] = device.Traits.ThermostatTemperatureSetpoint.Cool
					Tags["mode"] = "HEATCOOL"
				} else if device.Traits.ThermostatMode.Mode == "HEAT" {
					Fields["heat"] = device.Traits.ThermostatTemperatureSetpoint.Heat
					Tags["mode"] = "HEAT"
				} else if device.Traits.ThermostatMode.Mode == "COOL" {
					Fields["cool"] = device.Traits.ThermostatTemperatureSetpoint.Cool
					Tags["mode"] = "COOL"
				}

				if device.Traits.ThermostatHvac.Status == "OFF" {
					Fields["hvac"] = int8(0)
				} else {
					Fields["hvac"] = int8(1)
				}

				p := influxdb2.NewPoint(
					"nest",
					Tags,
					Fields,
					time.Now(),
				)

				err := writer.WritePoint(context.Background(), p)
				if err != nil {
					log.Println("ERROR: Could not write data point!")
					log.Print(err)
				} else {
					log.Printf("Wrote Nest thermostat metrics. Sleeping for %d minute(s).\n", config.NestConfig.Interval)
				}
			}
			time.Sleep(time.Minute * time.Duration(config.NestConfig.Interval))
		}
	}
}
