# go-thermostat-monitor
This is a small daemon for gathering thermostat data and comparing it to different weather sites. It writes to InfluxDB2.

> As much as I would like to continue maintaining this project, I moved into an apartment built in 1890 so I'm rocking radiators and portable ACs. Might I recommend [Home Assistant](https://www.home-assistant.io/)? It's far more user friendly with a lot more features. This I mostly wrote as a way to learn Go. Otherwise, feel free to fork it.

This is a port of my old [go-nest-temp-monitor](https://github.com/blakehartshorn/go-nest-temp-monitor) project to InfluxDB 2.0, with the addition of Ecobee support. Users on InfluxDB 1.x should seek out that project, although it is no longer maintained.

#### Celsius vs Fahrenheit
All metrics for all components of this app are collected in celsius because it is standard across most services. You can get fahrenheit in your influx queries and this works in Grafana as well:
```
from(bucket: "thermostat")
  |> range(start: v.timeRangeStart, stop: v.timeRangeStop)
  |> filter(fn: (r) => r["_measurement"] == "ecobee")
  |> filter(fn: (r) => r["_field"] == "temperature")
  |> map(fn: (r) => ({r with _value: (float(v: r._value) * 9.0 / 5.0 + 32.0)} ))
  |> last()
```

## Ecobee
You'll need an Ecobee Developer account. Get that setup here: https://www.ecobee.com/home/developer/loginDeveloper.jsp

After you're setup with that, go the Developer in the Ecobee web portal, "Create New" for an app key, and select PIN for the authorization method.

To get setup with a refresh token, you can use the interactive tutorial in Ecobee's API documentation. See here: https://www.ecobee.com/home/developer/api/examples/ex1.shtml

## Google Nest Device Access Sandbox
In order to make use of the Nest monitoring features, you need to sign up for Google's developer program and jump through a number of hoops. The documentation is here: https://developers.google.com/nest/device-access/registration

You'll need to get to the point where you have a client id, client secret and refresh token. The access of this token should be limited to this app. This will update the access_token every 45 minutes. 

Stats written to Influx include temperature, humidity, mode, heat/cool settings, device/parent relationship, and whether HVAC is currently running.

## Weather sites
3 weather sites are currently available to monitor. You can monitor one or all of them. Stats gather include temperature (celsius), humidity, and pressure.

### OpenWeatherMap
This weather source may be preferable as it's free, international, updates frequently, and supports frequent API calls. There is no way you'll go over the API limit using just this app, but I recommend not setting it to run too frequently out of courtesy. You can get the `cityid` value for the config by searching for your city and grabbing it from the URI. You can sign up and get an API key from here: https://home.openweathermap.org/users/sign_up

### AccuWeather
This works well, but the free tier is limited to 50 API calls a day, so you wont want to set this lower than 30 minutes for your check interval. You can run more frequent queries starting at $25/mo. It also assumes you're registering a commercial app even if you're a hobbyist. https://developer.accuweather.com/packages

You can get the `locationid` value by search for your city on the website and grabbing it from the URI.

### National Weather Service
This is a good free option for Americans, but it doesn't update very frequently. No API key or account are required. Search for your city on weather.gov and you should find the station code, e.g. KBOS for Boston. Place this in the config and this will just work. Data is written using the timestamp on the output and influxdb deduplicates this, so don't be surprised if you only get new data every hour or so.

## Installation
No build scripts are provided, but you can do the following:
```
mkdir -p ~/go/src
cd ~/go/src
git clone https://github.com/blakehartshorn/go-thermostat-monitor.git
cd go-thermostat-monitor
go build -o thermostat-monitor .
```
Copy the binary to where you would prefer to run it from.

This is presently tested on Debian Bullseye on both ARM64 and AMD64.

## Config
You'll need to create a bucket in influxdb for this app in advance. Mark the modules as true or false in config.yaml and add the appropriate API keys and station identifiers. Low intervals aren't that useful because the data provided isn't always new. The defaults shown in `config.yaml-example` are probably fine for most use cases.

## Running
```
Usage of ./thermostat-monitor:
  -c string
        Specify path to config.yaml (default "./config.yaml")
```
The only argument is the placement of config.yaml. An example systemd script:
```
[Unit]
Description=Thermostat Monitor
After=network.target influxdb.service
Requires=network.target

[Service]
User=username
Group=username
ExecStart=/home/username/thermostat-monitor/thermostat-monitor -c /home/username/thermostat-monitor/config.yaml
StandardOutput=syslog
StandardError=syslog
SyslogIdentifier=thermostat
Restart=on-failure

[Install]
WantedBy=multi-user.target
```
