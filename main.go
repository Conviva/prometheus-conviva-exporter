package main

import (
	"crypto/tls"
	"errors"
	"flag"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/buger/jsonparser"
	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	_ "net/http/pprof"
)

const namespace = "conviva_experience_insights"

var (
	tr = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client         = &http.Client{Transport: tr}
	listenAddress  = flag.String("web.listen-address", ":8080", "Address to listen on for telemetry")
	metricsPath    = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics")
	variableLabels = []string{"conviva_filter_id", "metriclens_dimension_value"}

	exporterUp = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "up"),
		"Indicates if the scrape is successful. 1=Success, 0=Fail", nil, nil,
	)

	// See https://developer.conviva.com/docs/metrics-api-v3/84ff2dc99dfeb-metrics for full list of metrics
	metrics = [10]string{
		"attempts",
		"video-start-failures",
		"exit-before-video-starts",
		"plays",
		"video-start-time",
		"rebuffering-ratio",
		"bitrate",
		"video-playback-failures",
		"ended-plays",
		"connection-induced-rebuffering-ratio",
	}

	metricDescriptions = map[string]*prometheus.Desc{
		"attempts": prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "attempts"),
			"Attempts counts all attempts to play a video which are initiated when a viewer clicks play or a video auto-plays.", variableLabels, nil,
		),
		"bitrate": prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "average_bitrate"),
			"Average bitrate calculates the bits played by the player. The bits played do not include bits in buffering or bits passed during paused video.", variableLabels, nil,
		),
		"connection_induced_rebuffering_ratio": prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "connection_induced_rebuffering_ratio"),
			"Connection Induced Rebuffering Ratio (CIRR) measures the percentage of total video viewing time (playTime plus all rebuffering) during which viewers experienced nonseek rebuffering.", variableLabels, nil,
		),
		"ended_plays": prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "ended_plays"),
			"An ended play is a play that ended during the selected interval. To count as an ended play, the viewing session must have at least one video frame that was viewed.", variableLabels, nil,
		),
		"exit_before_video_starts": prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "exits_before_video_start"),
			"Exits Before Video Start (EBVS) measures the Attempts that terminated before the video started, without a reported fatal error.", variableLabels, nil,
		),
		"plays": prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "plays"),
			"Plays (Successful Attempts) is counted when the viewer sees the first frame of video.", variableLabels, nil,
		),
		"rebuffering_ratio": prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "rebuffering_ratio"),
			"Rebuffering Ratio measures the percentage of total video viewing time (playTime + rebufferingTime) during which viewers experienced rebuffering.", variableLabels, nil,
		),
		"video_playback_failures": prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "video_playback_failures"),
			"Video playback failure occurs when video play terminates due to a playback error, such as video file corruption, insufficient streaming resources, or a sudden interruption in the video stream.", variableLabels, nil,
		),
		"video_start_failures": prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "video_start_failures"),
			"Video Start Failures (VSF) measures how often Attempts terminated during video startup before the first video frame was played, and a fatal error was reported.", variableLabels, nil,
		),
		"video_start_time": prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "video_start_time"),
			"Video Startup Time (VST) is the number of seconds between when the user clicks play or video auto-starts and when the first frame of a video is rendered.", variableLabels, nil,
		),
	}
)

// Metric has a name and a value
type Metric struct {
	metricName string
	value      float64
}

// NewDimension allocates and initializes a new Metric
func NewMetric() *Metric {
	metric := &Metric{}
	return metric
}

// Dimension represents a dimension and has a list of metrics
type Dimension struct {
	dimensionValue string
	metrics        []*Metric
}

// NewDimension allocates and initializes a new Dimension
func NewDimension() *Dimension {
	dimension := &Dimension{}
	return dimension
}

// MetricsData represents the API response
type MetricsData struct {
	filterTitle    string
	dimensionTitle string
	dimensions     []*Dimension
}

// NewMetricsData allocates and initializes a new QualityMetricLens
func NewMetricsData() *MetricsData {
	qualityMetriclensData := &MetricsData{}
	qualityMetriclensData.dimensions = make([]*Dimension, 0)
	return qualityMetriclensData
}

// Exporter is used to store metrics
type Exporter struct {
	convivaBaseURL, convivaAPIVersion, convivaClientID, convivaClientSecret, convivaFilterID, convivaDimensionName string
}

// NewExporter generates a new Exporter
func NewExporter(convivaBaseURL string, convivaAPIVersion string, convivaClientID string, convivaClientSecret string, convivaFilterID string, convivaDimensionName string) *Exporter {
	return &Exporter{
		convivaBaseURL:       convivaBaseURL,
		convivaAPIVersion:    convivaAPIVersion,
		convivaClientID:      convivaClientID,
		convivaClientSecret:  convivaClientSecret,
		convivaFilterID:      convivaFilterID,
		convivaDimensionName: convivaDimensionName,
	}
}

// Describe provides the Conviva metrics to prometheus.Describe
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range metricDescriptions {
		ch <- desc
	}
	ch <- exporterUp
}

// Collect is called by the Prometheus Client library when a scrape is peformed
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	qualityMetriclensData, err := e.getQualityMetriclens(ch)
	if err != nil {
		log.Println("Got error from Conviva API")
		log.Println(err)
		// Set flag to indicate failed scrape
		ch <- prometheus.MustNewConstMetric(exporterUp, prometheus.GaugeValue, 0)
		return
	}

	// Set flag to indicate successful scrape
	ch <- prometheus.MustNewConstMetric(exporterUp, prometheus.GaugeValue, 1)
	e.updateMetrics(ch, qualityMetriclensData)
}

// GetQualitySummaryAndUpdateMetrics calls the Conviva API and updates the metrics to Prometheus
func (e *Exporter) getQualityMetriclens(ch chan<- prometheus.Metric) (*MetricsData, error) {
	qualityMetriclensData := NewMetricsData()
	qualityMetriclensEndpoint := e.convivaBaseURL + "/insights/" + e.convivaAPIVersion + "/real-time-metrics/custom-selection/group-by/" + e.convivaDimensionName + "?minutes=2&granularity=PT1M&filter_id=" + e.convivaFilterID

	for i := 0; i < len(metrics); i++ {
		qualityMetriclensEndpoint += "&metric=" + metrics[i]
	}

	req, err := http.NewRequest("GET", qualityMetriclensEndpoint, nil)
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(e.convivaClientID, e.convivaClientSecret)

	// Make request and show output.
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}

	// Check if request failed
	if resp.StatusCode != 200 {
		reason, err := jsonparser.GetString(body, "name")
		if err == nil {
			err = errors.New("Invalid response from API. Reason: " + reason)
		}
		return nil, err
	}

	// Parse the dimension title
	dimensionName, err := jsonparser.GetString(body, "_meta", "group_by_dimension", "description")
	if err != nil {
		dimensionName = "Unknown"
	}
	qualityMetriclensData.dimensionTitle = dimensionName
	// Parse the filter title
	filterId, err := jsonparser.GetString(body, "_meta", "filter_info", "id")
	if err != nil {
		dimensionName = "Unknown"
	}
	qualityMetriclensData.filterTitle = filterId

	// For each dimension row in a filter
	jsonparser.ArrayEach(body, func(dimensionalDataRow []byte, dataType jsonparser.ValueType, offset int, err error) {
		if err != nil {
			log.Fatalln("Could not get time_series[0].dimensional_data")
			return
		}

		dimension := NewDimension()
		dimension.dimensionValue, err = jsonparser.GetString(dimensionalDataRow, "dimension", "value")
		if err != nil {
			log.Fatalln("Could not get time_series[0].dimensional_data.dimension.value")
			return
		}

		// For each metric in the dimension row
		jsonparser.ObjectEach(dimensionalDataRow, func(metricName []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
			if err != nil {
				log.Fatalln("Could not get time_series[0].dimensional_data.metrics")
				return nil
			}

			metric := NewMetric()
			metric.metricName = string(metricName)

			switch metric.metricName {
			case "attempts":
				metric.value, err = jsonparser.GetFloat(value, "count")
			case "bitrate":
				bps, err := jsonparser.GetFloat(value, "bps")
				if err != nil {
					metric.value = bps / 1000
				}
			case "connection_induced_rebuffering_ratio":
				metric.value, err = jsonparser.GetFloat(value, "ratio")
			case "ended_plays":
				metric.value, err = jsonparser.GetFloat(value, "count")
			case "exit_before_video_starts":
				metric.value, err = jsonparser.GetFloat(value, "percentage")
			case "plays":
				metric.value, err = jsonparser.GetFloat(value, "percentage")
			case "rebuffering_ratio":
				metric.value, err = jsonparser.GetFloat(value, "ratio")
			case "video_playback_failures":
				metric.value, err = jsonparser.GetFloat(value, "percentage")
			case "video_start_failures":
				metric.value, err = jsonparser.GetFloat(value, "percentage")
			case "video_start_time":
				metric.value, err = jsonparser.GetFloat(value, "value")
			default:
				break
			}

			dimension.metrics = append(dimension.metrics, metric)
			return nil
		}, "metrics")

		// dimension.metrics = metricsRow
		qualityMetriclensData.dimensions = append(qualityMetriclensData.dimensions, dimension)
	}, "time_series", "[0]", "dimensional_data")

	return qualityMetriclensData, nil
}

// UpdateMetrics reports all metrics to Prometheus
func (e *Exporter) updateMetrics(ch chan<- prometheus.Metric, data *MetricsData) {
	// Set all the metrics to Prometheus. Iterate all dimensions
	for j := 0; j < len(data.dimensions); j++ {
		filterTitle := data.filterTitle
		dimension := data.dimensions[j]

		// Iterate all metrics in the dimension
		for k := 0; k < len(dimension.metrics); k++ {
			metric := dimension.metrics[k]
			dimensionValue := dimension.dimensionValue
			ch <- prometheus.MustNewConstMetric(
				metricDescriptions[metric.metricName], prometheus.GaugeValue, metric.value, filterTitle, dimensionValue,
			)
		}
	}
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Error loading .env file, assume env variables are set.")
	}

	flag.Parse()

	convivaBaseURL := os.Getenv("CONVIVA_BASE_URL")
	convivaAPIVersion := os.Getenv("CONVIVA_API_VERSION")
	convivaClientID := os.Getenv("CONVIVA_CLIENT_ID")
	convivaClientSecret := os.Getenv("CONVIVA_CLIENT_SECRET")
	convivaFilterID := os.Getenv("CONVIVA_FILTER_ID")
	convivaDimensionName := os.Getenv("CONVIVA_DIMENSION_NAME")

	if convivaBaseURL == "" {
		log.Fatal("Error loading convivaBaseURL from env variables. Exiting.")
	}

	exporter := NewExporter(convivaBaseURL, convivaAPIVersion, convivaClientID, convivaClientSecret, convivaFilterID, convivaDimensionName)
	prometheus.MustRegister(exporter)

	http.Handle(*metricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
	         <head><title>Conviva Experience Insights Prometheus Exporter</title></head>
	         <body>
	         <h1>Conviva Experience Insights Quality Summary Exporter</h1>
	         <p><a href='` + *metricsPath + `'>Metrics</a></p>
	         </body>
	         </html>`))
	})
	log.Fatal(http.ListenAndServe(*listenAddress, nil))
}
