package main

import (
	"crypto/tls"
	"errors"
	"flag"
	"io/ioutil"
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

	metricDescriptions = []*prometheus.Desc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "attempts"),
			"Attempts counts all attempts to play a video which are initiated when a viewer clicks play or a video auto-plays.", variableLabels, nil,
		),
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "video_start_failures"),
			"Video Start Failures (VSF) measures how often Attempts terminated during video startup before the first video frame was played, and a fatal error was reported.", variableLabels, nil,
		),
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "exits_before_video_start"),
			"Exits Before Video Start (EBVS) measures the Attempts that terminated before the video started, without a reported fatal error.", variableLabels, nil,
		),
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "plays"),
			"Plays (Successful Attempts) is counted when the viewer sees the first frame of video.", variableLabels, nil,
		),
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "video_start_time"),
			"Video Startup Time (VST) is the number of seconds between when the user clicks play or video auto-starts and when the first frame of a video is rendered.", variableLabels, nil,
		),
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "rebuffering_ratio"),
			"Rebuffering Ratio measures the percentage of total video viewing time (playTime + rebufferingTime) during which viewers experienced rebuffering.", variableLabels, nil,
		),
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "average_bitrate"),
			"Average bitrate calculates the bits played by the player. The bits played do not include bits in buffering or bits passed during paused video.", variableLabels, nil,
		),
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "video_playback_failures"),
			"Video playback failure occurs when video play terminates due to a playback error, such as video file corruption, insufficient streaming resources, or a sudden interruption in the video stream.", variableLabels, nil,
		),
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "ended_plays"),
			"An ended play is a play that ended during the selected interval. To count as an ended play, the viewing session must have at least one video frame that was viewed.", variableLabels, nil,
		),
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "connection_induced_rebuffering_ratio"),
			"Connection Induced Rebuffering Ratio (CIRR) measures the percentage of total video viewing time (playTime plus all rebuffering) during which viewers experienced nonseek rebuffering.", variableLabels, nil,
		),
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "video_restart_time"),
			"Video Restart Time (VRT) is the number of seconds after user-initiated seeking until video begins playing.", variableLabels, nil,
		),
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "video_start_failures_technical"),
			"Video Start Failures (VSF-T) measures how often Attempts terminated during video startup before the first video frame was played, and a fatal error was reported due to a technical issue, such as prolonged buffering.", variableLabels, nil,
		),
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "video_start_failures_business"),
			"VSFs with at least one error message that matches the configured business errors are counted in the Video Start Failures Business (VPF-B) metric.", variableLabels, nil,
		),
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "video_playback_failures_technical"),
			"VPFs with errors that match the configured business errors are not counted as VPF-T, but count towards the Video Playback Failures Business (VPF-B) metric.", variableLabels, nil,
		),
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "video_playback_failures_business"),
			"VPFs with at least one error message that matches the configured business errors are counted in the Video Playback Failures Business (VPF-B) metric.", variableLabels, nil,
		),
	}
)

// Dimension represents a dimension and has a list of metrics
type Dimension struct {
	metrics []float64
}

// NewDimension allocates and initializes a new Dimension
func NewDimension() *Dimension {
	dimension := &Dimension{}
	return dimension
}

// FilterTable represents a filter and keeps tables of metrics, grouped by dimensions
type FilterTable struct {
	filterID   string
	dimensions []*Dimension
}

// NewFilterTable allocates and initializes a new FilterTable
func NewFilterTable(filterID string) *FilterTable {
	filterTable := &FilterTable{
		filterID: filterID,
	}
	filterTable.dimensions = make([]*Dimension, 0)
	return filterTable
}

// QualityMetricLensData represents the API response
type QualityMetricLensData struct {
	filters         []*FilterTable
	dimensionTitles []string
}

// NewQualityMetricLensData allocates and initializes a new QualityMetricLens
func NewQualityMetricLensData() *QualityMetricLensData {
	qualityMetriclensData := &QualityMetricLensData{}
	qualityMetriclensData.filters = make([]*FilterTable, 0)
	qualityMetriclensData.dimensionTitles = make([]string, 0)
	return qualityMetriclensData
}

// Exporter is used to store metrics
type Exporter struct {
	convivaBaseURL, convivaAPIVersion, convivaClientID, convivaClientSecret, convivaFilterIDs, convivaDimensionID string
}

// NewExporter generates a new Exporter
func NewExporter(convivaBaseURL string, convivaAPIVersion string, convivaClientID string, convivaClientSecret string, convivaFilterIDs string, convivaDimensionID string) *Exporter {
	return &Exporter{
		convivaBaseURL:      convivaBaseURL,
		convivaAPIVersion:   convivaAPIVersion,
		convivaClientID:     convivaClientID,
		convivaClientSecret: convivaClientSecret,
		convivaFilterIDs:    convivaFilterIDs,
		convivaDimensionID:  convivaDimensionID,
	}
}

// Describe provides the Conviva metrics to prometheus.Describe
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	for i := 0; i < len(metricDescriptions); i++ {
		ch <- metricDescriptions[i]
	}
	ch <- exporterUp
}

// Collect is called by the Prometheus Client library when a scrape is peformed
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	qualityMetriclensData, err := e.getQualityMetriclens(ch)
	if err != nil {
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
func (e *Exporter) getQualityMetriclens(ch chan<- prometheus.Metric) (*QualityMetricLensData, error) {
	qualityMetriclensData := NewQualityMetricLensData()
	qualityMetriclensEndpoint := e.convivaBaseURL + "/insights/" + e.convivaAPIVersion + "/metrics.json?metrics=quality_metriclens&filter_ids=" + e.convivaFilterIDs + "&metriclens_dimension_id=" + e.convivaDimensionID

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

	body, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}

	// Check if request failed
	if resp.StatusCode != 200 {
		reason, err := jsonparser.GetString(body, "reason")
		if err == nil {
			err = errors.New("Invalid response from API. Reason: " + reason)
		}
		return nil, err
	}

	// Check if filter is warming up
	isWarmingUp := false
	jsonparser.ArrayEach(body, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
		if err == nil {
			isWarmingUp = true
		}
	}, "quality_metriclens", "meta", "filters_warmup")

	if isWarmingUp {
		err := errors.New("filter is warming up")
		return nil, err
	}

	// Parse all dimension titles
	jsonparser.ArrayEach(body, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
		if err == nil {
			qualityMetriclensData.dimensionTitles = append(qualityMetriclensData.dimensionTitles, string(value))
		}
	}, "quality_metriclens", "xvalues")

	// For each filter, parse all the metrics
	jsonparser.ObjectEach(body, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
		// log.Println("Key: '%s'\n Value: '%s'\n Type: %s\n", string(key), string(value), dataType)

		filterID := string(key)
		filterTable := NewFilterTable(filterID)

		// For each dimension row in a filter
		jsonparser.ArrayEach(body, func(dimensionRow []byte, dataType jsonparser.ValueType, offset int, err error) {
			if err != nil {
				return
			}
			dimension := NewDimension()
			metricsRow := []float64{}
			// For each metric in the dimension row
			jsonparser.ArrayEach(dimensionRow, func(metricVal []byte, dataType jsonparser.ValueType, offset int, err error) {
				if err != nil {
					return
				}
				metricFloat, err := jsonparser.GetFloat(metricVal)
				if err != nil {
					return
				}
				metricsRow = append(metricsRow, metricFloat)
			})
			dimension.metrics = metricsRow
			filterTable.dimensions = append(filterTable.dimensions, dimension)
		}, "quality_metriclens", "tables", filterID, "rows")

		qualityMetriclensData.filters = append(qualityMetriclensData.filters, filterTable)

		return nil
	}, "quality_metriclens", "tables")

	return qualityMetriclensData, nil
}

// UpdateMetrics reports all metrics to Prometheus
func (e *Exporter) updateMetrics(ch chan<- prometheus.Metric, data *QualityMetricLensData) {
	// Set all the metrics to Prometheus. First, iterate all filters.
	for i := 0; i < len(data.filters); i++ {
		filter := data.filters[i]

		// Iterate all dimensions
		for j := 0; j < len(filter.dimensions); j++ {
			dimensionTitle := data.dimensionTitles[j]
			dimension := filter.dimensions[j]

			// Iterate all metrics in the dimension
			for k := 0; k < len(dimension.metrics); k++ {
				metricValue := dimension.metrics[k]
				ch <- prometheus.MustNewConstMetric(
					metricDescriptions[k], prometheus.GaugeValue, metricValue, filter.filterID, dimensionTitle,
				)
			}
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
	convivaFilterIDs := os.Getenv("CONVIVA_FILTER_IDS")
	convivaDimensionID := os.Getenv("CONVIVA_DIMENSION_ID")

	if convivaBaseURL == "" {
		log.Fatal("Error loading convivaBaseURL from env variables. Exiting.")
	}

	exporter := NewExporter(convivaBaseURL, convivaAPIVersion, convivaClientID, convivaClientSecret, convivaFilterIDs, convivaDimensionID)
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
