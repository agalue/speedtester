package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type LatencyStats struct {
	IQM    float64 `json:"iqm"`
	Low    float64 `json:"low"`
	High   float64 `json:"high"`
	Jitter float64 `json:"jitter"`
}

type BandwidthStats struct {
	Bandwidth int           `json:"bandwidth"`
	Bytes     int           `json:"bytes"`
	Elapsed   int           `json:"elapsed"`
	Latency   *LatencyStats `json:"latency"`
}

func (s *BandwidthStats) GetBandWithInMbps() float64 {
	return float64(s.Bandwidth) * 0.000008
}

type PingStats struct {
	Jitter  float64 `json:"jitter"`
	Latency float64 `json:"latency"`
	Low     float64 `json:"low"`
	High    float64 `json:"high"`
}

type ServerInfo struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Location string `json:"location"`
}

func (s *ServerInfo) GetID() string {
	return strconv.Itoa(s.ID)
}

type Stats struct {
	Server     *ServerInfo     `json:"server"`
	Ping       *PingStats      `json:"ping"`
	Download   *BandwidthStats `json:"download"`
	Upload     *BandwidthStats `json:"upload"`
	PacketLoss float64         `json:"packetLoss"`
	ISP        string          `json:"isp"`
}

func (s *Stats) HasError() error {
	if s.Server == nil {
		return fmt.Errorf("missing server details")
	}
	if s.Ping == nil {
		return fmt.Errorf("missing ping details")
	}
	if s.Download == nil {
		return fmt.Errorf("missing download details")
	}
	if s.Download.Latency == nil {
		return fmt.Errorf("missing download latency details")
	}
	if s.Upload == nil {
		return fmt.Errorf("missing upload details")
	}
	if s.Upload.Latency == nil {
		return fmt.Errorf("missing upload latency details")
	}
	return nil
}

func (s *Stats) Log() {
	if s.HasError() != nil {
		return
	}
	log.Printf("Server %d: %s (ISP: %s)", s.Server.ID, s.Server.Name, s.ISP)
	log.Printf("Download %.2f Mbps (latency: %.2f/%.2f ms, jitter: %.2f ms)", s.Download.GetBandWithInMbps(), s.Download.Latency.IQM, s.Download.Latency.High, s.Download.Latency.Jitter)
	log.Printf("Upload %.2f Mbps (latency: %.2f/%.2f ms, jitter: %.2f ms)", s.Upload.GetBandWithInMbps(), s.Upload.Latency.IQM, s.Upload.Latency.High, s.Upload.Latency.Jitter)
	log.Printf("Ping %.2f/%.2f ms (jitter: %.2f ms)", s.Ping.Latency, s.Ping.High, s.Ping.Jitter)
}

type PrometheusStats struct {
	DownloadBandwidth *prometheus.GaugeVec
	DownloadLatency   *prometheus.GaugeVec
	DownloadJitter    *prometheus.GaugeVec
	UploadBandwidth   *prometheus.GaugeVec
	UploadLatency     *prometheus.GaugeVec
	UploadJitter      *prometheus.GaugeVec
	PingLatency       *prometheus.GaugeVec
	PingJitter        *prometheus.GaugeVec
	PacketLoss        *prometheus.GaugeVec
	Requests          *prometheus.CounterVec
}

func (s *PrometheusStats) Init() {
	s.Requests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "speedtest_total_requests",
		Help: "The total number of requests",
	}, []string{"status"})

	s.DownloadBandwidth = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "speedtest_download_speed",
		Help: "The Download Rate in Mbps",
	}, []string{"isp", "server_id", "server_name", "server_location"})
	s.DownloadLatency = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "speedtest_download_latency",
		Help: "The Download Latency in milliseconds (iqm, low, high)",
	}, []string{"isp", "server_id", "server_name", "server_location", "latency"})
	s.DownloadJitter = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "speedtest_download_jitter",
		Help: "The Download Jitter in milliseconds",
	}, []string{"isp", "server_id", "server_name", "server_location"})

	s.UploadBandwidth = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "speedtest_upload_speed",
		Help: "The Upload Rate in Mbps",
	}, []string{"isp", "server_id", "server_name", "server_location"})
	s.UploadLatency = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "speedtest_upload_latency",
		Help: "The Upload Latency in milliseconds (iqm, low, high)",
	}, []string{"isp", "server_id", "server_name", "server_location", "latency"})
	s.UploadJitter = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "speedtest_upload_jitter",
		Help: "The Upload Jitter in milliseconds",
	}, []string{"isp", "server_id", "server_name", "server_location"})

	s.PingLatency = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "speedtest_ping_latency",
		Help: "The Ping Latency in milliseconds (iqm, low, high)",
	}, []string{"isp", "server_id", "server_name", "server_location", "latency"})
	s.PingJitter = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "speedtest_ping_jitter",
		Help: "The Ping Jitter in milliseconds",
	}, []string{"isp", "server_id", "server_name", "server_location"})

	s.PacketLoss = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "speedtest_packet_loss",
		Help: "The Number of Packet Loss",
	}, []string{"isp", "server_id", "server_name", "server_location"})

	prometheus.MustRegister(
		s.Requests,
		s.DownloadBandwidth,
		s.DownloadLatency,
		s.DownloadJitter,
		s.UploadBandwidth,
		s.UploadLatency,
		s.UploadJitter,
		s.PingLatency,
		s.PingJitter,
		s.PacketLoss,
	)
}

func (s *PrometheusStats) Update(stats *Stats) {
	if stats.HasError() != nil {
		return
	}

	c := stats.Server

	s.DownloadBandwidth.WithLabelValues(stats.ISP, c.GetID(), c.Name, c.Location).Set(stats.Download.GetBandWithInMbps())
	s.DownloadLatency.WithLabelValues(stats.ISP, c.GetID(), c.Name, c.Location, "iqm").Set(stats.Download.Latency.IQM)
	s.DownloadLatency.WithLabelValues(stats.ISP, c.GetID(), c.Name, c.Location, "low").Set(stats.Download.Latency.Low)
	s.DownloadLatency.WithLabelValues(stats.ISP, c.GetID(), c.Name, c.Location, "high").Set(stats.Download.Latency.High)
	s.DownloadJitter.WithLabelValues(stats.ISP, c.GetID(), c.Name, c.Location).Set(stats.Download.Latency.Jitter)

	s.UploadBandwidth.WithLabelValues(stats.ISP, c.GetID(), c.Name, c.Location).Set(stats.Upload.GetBandWithInMbps())
	s.UploadLatency.WithLabelValues(stats.ISP, c.GetID(), c.Name, c.Location, "iqm").Set(stats.Upload.Latency.IQM)
	s.UploadLatency.WithLabelValues(stats.ISP, c.GetID(), c.Name, c.Location, "low").Set(stats.Upload.Latency.Low)
	s.UploadLatency.WithLabelValues(stats.ISP, c.GetID(), c.Name, c.Location, "high").Set(stats.Upload.Latency.High)
	s.UploadJitter.WithLabelValues(stats.ISP, c.GetID(), c.Name, c.Location).Set(stats.Upload.Latency.Jitter)

	s.PingLatency.WithLabelValues(stats.ISP, c.GetID(), c.Name, c.Location, "iqm").Set(stats.Ping.Latency)
	s.PingLatency.WithLabelValues(stats.ISP, c.GetID(), c.Name, c.Location, "low").Set(stats.Ping.Low)
	s.PingLatency.WithLabelValues(stats.ISP, c.GetID(), c.Name, c.Location, "high").Set(stats.Ping.High)
	s.PingJitter.WithLabelValues(stats.ISP, c.GetID(), c.Name, c.Location).Set(stats.Ping.Jitter)

	s.PacketLoss.WithLabelValues(stats.ISP, c.GetID(), c.Name, c.Location).Set(stats.PacketLoss)
}

type SpeedTester struct {
	Command   string
	ServerID  int
	promStats *PrometheusStats
}

func (t *SpeedTester) Run() error {
	log.Println("Starting speed test")

	status := "error"
	defer func() {
		t.promStats.Requests.WithLabelValues(status).Inc()
	}()

	if t.Command == "" {
		t.Command = "/usr/bin/speedtest"
	}
	if t.promStats == nil {
		t.promStats = new(PrometheusStats)
		t.promStats.Init()
	}

	start := time.Now()

	args := []string{"--accept-license", "--progress=no", "--format=json"}
	if t.ServerID > 0 {
		log.Printf("Using Server ID %d", t.ServerID)
		args = append(args, []string{"--server-id", strconv.Itoa(t.ServerID)}...)
	}
	cmd := exec.Command(t.Command, args...)
	out := new(bytes.Buffer)
	cmd.Stdout = out

	if err := cmd.Run(); err != nil {
		return err
	}

	stats := new(Stats)
	if err := json.Unmarshal(out.Bytes(), stats); err != nil {
		return err
	}

	stats.Log()
	elapsed := time.Since(start)
	log.Printf("Finished in %s", elapsed.String())
	if err := stats.HasError(); err != nil {
		return err
	}
	t.promStats.Update(stats)
	status = "ok"
	return nil
}

func main() {
	var prometheusPort int
	var updateFrequency time.Duration
	runner := new(SpeedTester)

	flag.IntVar(&prometheusPort, "port", 8080, "HTTP Port to expose statistics via Prometheus")
	flag.DurationVar(&updateFrequency, "frequency", 15*time.Minute, "Frequency on which statistics are retrieved and proceessed")
	flag.IntVar(&runner.ServerID, "server", 0, "Ookla Server ID (must be listed on the output of 'speedtest --servers')")
	flag.StringVar(&runner.Command, "path", "/usr/bin/speedtest", "Ookla Speed Test CLI Path ID")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	defer func() {
		signal.Stop(signalChan)
	}()

	go func() {
		log.Printf("Starting Prometheus Metrics server on port %d", prometheusPort)
		http.Handle("/", promhttp.Handler())
		err := http.ListenAndServe(fmt.Sprintf(":%d", prometheusPort), nil)
		if err != nil {
			log.Fatalf("Cannot start prometheus HTTP server: %v", err)
		}
	}()

	go func() {
		log.Printf("Statistics will be collected and processed every %s", updateFrequency.String())
		err := runner.Run()
		if err != nil {
			log.Printf("cannot execute command: %v", err)
		}
		ticker := time.NewTicker(updateFrequency)
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
			case <-ticker.C:
				err := runner.Run()
				if err != nil {
					log.Printf("cannot execute command: %v", err)
				}
			}
		}
	}()

	<-signalChan
	cancel()
	log.Println("Good bye")
}
