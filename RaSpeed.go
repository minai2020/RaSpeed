package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron"
	log "github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v2"
)

// Config is yaml rep of the config file there is loaded in
type Config struct {
	UseDocker          string `yaml:"use-docker"`
	DockerImage        string `yaml:"docker-image"`
	GrafanaAddress     string `yaml:"grafana-address"`
	GrafanaPort        string `yaml:"grafana-port"`
	TsdbDownloadPrefix string `yaml:"tsdb-download-prefix"`
	TsdbUploadPrefix   string `yaml:"tsdb-upload-prefix"`
	LoggingFile        string `yaml:"logging-File"`
	Servers            []struct {
		HostID     string `yaml:"Host-ID"`
		ServerIP   string `yaml:"server-IP"`
		ServerPort string `yaml:"server-port"`
		Testlength int    `yaml:"testlength"`
		Crontask   string `yaml:"crontask"`
		Parallel   int    `yaml:"parallel"`
	} `yaml:"Servers"`
}

// Iperf3Json type is export from
type Iperf3Json struct {
	Start struct {
		Connected []struct {
			LocalHost  string `json:"local_host"`
			RemoteHost string `json:"remote_host"`
		} `json:"connected"`
		Version    string `json:"version"`
		SystemInfo string `json:"system_info"`
		Timestamp  struct {
			Time     string `json:"time"`
			Timesecs int    `json:"timesecs"`
		} `json:"timestamp"`
		ConnectingTo struct {
			Host string `json:"host"`
			Port int    `json:"port"`
		} `json:"connecting_to"`
		TCPMssDefault int `json:"tcp_mss_default"`
		TestStart     struct {
			Protocol   string `json:"protocol"`
			NumStreams int    `json:"num_streams"`
			Blksize    int    `json:"blksize"`
			Omit       int    `json:"omit"`
			Duration   int    `json:"duration"`
			Bytes      int    `json:"bytes"`
			Blocks     int    `json:"blocks"`
			Reverse    int    `json:"reverse"`
			Tos        int    `json:"tos"`
		} `json:"test_start"`
	} `json:"start"`
	Intervals []struct {
		Streams []struct {
			Socket        int     `json:"socket"`
			Start         int     `json:"start"`
			End           float64 `json:"end"`
			Seconds       float64 `json:"seconds"`
			Bytes         int     `json:"bytes"`
			BitsPerSecond float64 `json:"bits_per_second"`
			Omitted       bool    `json:"omitted"`
			Sender        bool    `json:"sender"`
		} `json:"streams"`
		Sum struct {
			Start         int     `json:"start"`
			End           float64 `json:"end"`
			Seconds       float64 `json:"seconds"`
			Bytes         int     `json:"bytes"`
			BitsPerSecond float64 `json:"bits_per_second"`
			Omitted       bool    `json:"omitted"`
			Sender        bool    `json:"sender"`
		} `json:"sum"`
	} `json:"intervals"`
	End struct {
		Streams []struct {
			Sender struct {
				Socket        int     `json:"socket"`
				Start         int     `json:"start"`
				End           float64 `json:"end"`
				Seconds       float64 `json:"seconds"`
				Bytes         int     `json:"bytes"`
				BitsPerSecond float64 `json:"bits_per_second"`
				Sender        bool    `json:"sender"`
			} `json:"sender"`
			Receiver struct {
				Socket        int     `json:"socket"`
				Start         int     `json:"start"`
				End           float64 `json:"end"`
				Seconds       float64 `json:"seconds"`
				Bytes         int     `json:"bytes"`
				BitsPerSecond float64 `json:"bits_per_second"`
				Sender        bool    `json:"sender"`
			} `json:"receiver"`
		} `json:"streams"`
		SumSent struct {
			Start         int     `json:"start"`
			End           float64 `json:"end"`
			Seconds       float64 `json:"seconds"`
			Bytes         int     `json:"bytes"`
			BitsPerSecond float64 `json:"bits_per_second"`
			Sender        bool    `json:"sender"`
		} `json:"sum_sent"`
		SumReceived struct {
			Start         int     `json:"start"`
			End           float64 `json:"end"`
			Seconds       float64 `json:"seconds"`
			Bytes         int     `json:"bytes"`
			BitsPerSecond float64 `json:"bits_per_second"`
			Sender        bool    `json:"sender"`
		} `json:"sum_received"`
		CPUUtilizationPercent struct {
			HostTotal    float64 `json:"host_total"`
			HostUser     float64 `json:"host_user"`
			HostSystem   float64 `json:"host_system"`
			RemoteTotal  float64 `json:"remote_total"`
			RemoteUser   float64 `json:"remote_user"`
			RemoteSystem float64 `json:"remote_system"`
		} `json:"cpu_utilization_percent"`
		ReceiverTCPCongestion string `json:"receiver_tcp_congestion"`
	} `json:"end"`
	Error string `json:"error"`
}

// Cli ommandline options
type Cli struct {
	Debug      bool
	ConfigPath string
	Help       bool
	LogFile    string
	LogLevel   string
}

var cli *Cli
var config Config
var mutex = &sync.Mutex{}

func init() {

	const (
		debugFlag       = true
		debugDescrip    = "Run in debug mode to display all shell commands being executed"
		configPath      = "./config.yaml"
		configDescrip   = "Path to the configuration file -config=path/config.yml"
		helpFlag        = false
		helpDescrip     = "Print Usage Options (this)"
		logFile         = "logfile.log"
		logFlagDescrip  = "Log to file "
		logLevel        = "Trace"
		logLevelDescrip = "Trace, Debug, Info, Warning, Error, Fatal, Panic"
	)
	cli = &Cli{}
	flag.BoolVar(&cli.Debug, "debug", debugFlag, debugDescrip)
	flag.StringVar(&cli.ConfigPath, "config", configPath, configDescrip)
	flag.BoolVar(&cli.Help, "help", helpFlag, helpDescrip)
	flag.StringVar(&cli.LogLevel, "Loglevel", logLevel, logFlagDescrip)
	flag.StringVar(&cli.LogFile, "logfile", logFile, logFlagDescrip)

	if cli.Debug {
		log.WithFields(log.Fields{
			"Debug":      cli.Debug,
			"Loglevel":   logLevel,
			"ConfigPath": configPath,
		}).Info(debugDescrip)

	}

	level, err := log.ParseLevel(cli.LogLevel)
	if err != nil {
		log.Warn("Cannot parse conf level, default to INFO")
	} else {
		log.Println(level)
		log.SetLevel(level)
	}
	if cli.LogFile != "" {
		log.Info("Logging will continue on file : " + cli.LogFile)
		f, err := os.OpenFile(cli.LogFile, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0666)
		if err != nil {
			fmt.Printf("error opening file: %v", err)
		}

		log.SetOutput(f)
	}

}

func main() {

	println("Program start")

	data, err := ioutil.ReadFile(cli.ConfigPath)
	if err != nil {
		log.Fatalln("There was a problem opening the configuration file. Make sure "+
			"'config.yml' is located in the same directory as the binary 'raspeed' or set"+
			" the location using -config=path/config.yml || Error: ", err)
	}
	//config := Config{}

	if err := yaml.Unmarshal([]byte(data), &config); err != nil {
		log.Fatal(err)
	}
	println("Servers the wille be tested. ")

	for index := range config.Servers {

		log.WithFields(log.Fields{
			"Index":       index,
			"Cron":        config.Servers[index].Crontask,
			"Host ID":     config.Servers[index].HostID,
			"Server IP":   config.Servers[index].ServerIP,
			"Server Port": config.Servers[index].ServerPort,
			"Parallel":    config.Servers[index].Parallel,
			"Test in sec": config.Servers[index].Testlength,
		}).Info("Adding host to task list")

		cronDockeDuplex(config.Servers[index].Crontask,
			config.DockerImage,
			config.Servers[index].ServerIP,
			config.Servers[index].ServerPort,
			config.Servers[index].Parallel,
			config.Servers[index].Testlength,
			config.Servers[index].HostID)

	}

	println("Out of Cron assigment")
	time.Sleep(time.Minute * 3)
	println("Program should have ended")
	//fmt.Printf("%v ", v.NumField())

	/**
	up, down := runDockerIperfDuplex("probez/iperf3", "speedtest.hiper.dk", 3, 5)

	grafUpload("192.168.10.200", "2003", "bandwidth.upload", "Realhost", down)
	grafUpload("192.168.10.200", "2003", "bandwidth.download", "Realhost", up)
	fmt.Println("Program is exited")
	*/

	/*****-----

		fmt.Println("Program is in progress")
		log.Println("STARTIGN CRON PROCESS")
	  /**/
	//cronDockeDuplex("*/1 * * * *", "probez/iperf3", "speedtest.hiper.dk", "5201", 3, 5, "hostid-update")

	//time.Sleep(time.Minute * 3)
}

func cronDockeDuplex(timestring string, dockerimage string, server string, port string, parallel int, howlong int, hostid string) {
	c := cron.New()
	c.AddFunc(timestring, func() { runDockerDuplex(dockerimage, server, port, parallel, howlong, hostid) })
	c.Start()

}

// RunDockerDuplex will run test both ways
func runDockerDuplex(dockerimage string, server string, port string, parallel int, howlong int, hostid string) {
	up, down := runDockerIperfDuplex(dockerimage, server, port, parallel, howlong)

	grafUpload(config.GrafanaAddress, config.GrafanaPort, config.TsdbDownloadPrefix, hostid, down)
	grafUpload(config.GrafanaAddress, config.GrafanaPort, config.TsdbUploadPrefix, hostid, up)

}

func runDockerIperfDuplex(dockerimage string, server string, port string, parallel int, howlong int) (upload float64, download float64) {

	var uploadresult, err = runDockerIperf(dockerimage, server, port, false, parallel, howlong)
	if err != nil {
		log.Error("Upload failed")
	}
	upload = (uploadresult.End.SumReceived.BitsPerSecond / 1000000)

	log.Printf("Upload speed meassured : %.2f Mbit/s", upload)

	var downloadresult, erro = runDockerIperf(dockerimage, server, port, true, parallel, howlong)
	if erro != nil {
		log.Error("Download failed")
	}
	download = (downloadresult.End.SumReceived.BitsPerSecond / 1000000)
	log.Printf("Download speed meassured : %.2f Mbit/s", download)

	return upload, download

}

func grafUpload(graphitehost string, graphiteport string, graphiteTableprefix string, graphiteendpointname string, result float64) {
	graphiteSocket := net.JoinHostPort(graphitehost, graphiteport)
	timeNow := time.Now().Unix()
	uploadResults := fmt.Sprintf("%3f", result)
	msg := fmt.Sprintf("%s.%s %s %d\n", graphiteTableprefix, graphiteendpointname, uploadResults, timeNow)
	log.WithFields(log.Fields{
		"graphiteTableprefix":  graphiteTableprefix,
		"graphiteendpointname": graphiteendpointname,
		"Upload results":       uploadResults,
		"time (local)":         timeNow,
	}).Trace("Function grafUpload called with")

	log.Trace("String to update : ", msg)
	conn, err := net.Dial("tcp", graphiteSocket)

	if err != nil {

		log.Printf("Could not connect to the graphite server @%s", graphiteSocket)
		log.Printf("Verify the graphite server is running and reachable at %s", graphitehost)
	} else {
		defer conn.Close()
		_, err = fmt.Fprintf(conn, msg)

		log.WithFields(log.Fields{
			"Remote Host": conn.RemoteAddr(),
			"msg":         msg,
		}).Info("Success uploading to Graphite host")

		if err != nil {
			log.Printf("Error writing to the graphite server at -> %s", graphiteSocket)
		}

	}
}

func runDockerIperf(dockerimage string, server string, port string, reverse bool, parallel int, howlong int) (iperfResult Iperf3Json, err error) {
	//lets get some silince
	log.Trace("Mutex lock")
	mutex.Lock()

	data := Iperf3Json{}
	path, err := exec.LookPath("docker")
	if err != nil {
		log.Fatalln("installing docker is needed")
	}

	var res string

	if reverse {
		res = "-R"
	}

	timeout := time.Duration(int64(howlong*2)) * time.Second

	log.Printf("Function will be called with a timeout of :%v", (time.Duration(int64(howlong*2)) * time.Second))

	// Create a new context and add a timeout so command will timeout after 2*test duration
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel() // The cancel should be deferred so resources are cleaned up	// Create a new context and add a timeout to it

	//cmd := exec.CommandContext(ctx, "ping", "-c 4", "-i 1", "8.8.8.8")
	cmd := exec.CommandContext(ctx, path, "run", "-i", "--rm", dockerimage, "-c", server, "-J", res, "-P", strconv.Itoa(parallel), "-p", port, "-t", strconv.Itoa(howlong))
	// Relax off again

	log.Trace(cmd)
	output, err := cmd.Output()

	if err != nil {

		if strings.Contains(err.Error(), "125") {
			log.Fatal("Docker is not running !!")
		}

		if ctx.Err() == context.DeadlineExceeded {

			log.Error("Command failed do to time out after")
			return data, errors.New("Command failed do to time out after")
		}
		// It was a fail, most likely because of busy serer
		// Error is also comminig on Iperf3Json
		_ = json.Unmarshal(output, &data)

		log.Error("IPerf server: " + server + " reported back :" + data.Error)

		err = errors.New(data.Error)

		//	fmt.Printf("Return data: %s\n", output)
		log.Trace("Mutex Unlock")
		mutex.Unlock()
		return data, err

	}

	_ = json.Unmarshal(output, &data)

	//fmt.Printf("Return data: %s\n", output)
	log.Trace("Remove Mutex")
	mutex.Unlock()
	return data, err

}
