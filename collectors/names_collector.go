package collectors

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"regexp"
	"time"
	"os"
	"bufio"
)

type NamesCollector struct {
	namespace   string
	namesMetric prometheus.CounterVec
	totalMetric prometheus.Counter
	stats       map[string]float64

	scrapesTotalMetric              prometheus.Counter
	scrapeErrorsTotalMetric         prometheus.Counter
	lastScrapeErrorMetric           prometheus.Gauge
	lastScrapeTimestampMetric       prometheus.Gauge
	lastScrapeDurationSecondsMetric prometheus.Gauge
}

func NewNamesCollector(namespace string, sender *chan string, includeFile string, excludeFile string) *NamesCollector {
	stats := make(map[string]float64)
	include := make(map[string]bool)
	exclude := make(map[string]bool)

	if "" != includeFile {
		log.Infoln("Will only export names that ARE in the file ", includeFile)
		err := makeList(includeFile, &include)
		if err != nil {
			log.Errorln("Failed to use include file: ", includeFile, err)
		}
	}
	if "" != excludeFile {
		log.Infoln("Will only export names that ARE NOT the file ", excludeFile)
		err := makeList(excludeFile, &exclude)
		if err != nil {
			log.Errorln("Failed to use exclude file: ", excludeFile, err)
		}
	}


	/* Spin off a thread that will gather our data on every read from the file */
	go func(sender *chan string, stats *map[string]float64, inc *map[string]bool, exc *map[string]bool) {
		//22-Mar-2020 14:54:27.568 queries: info: client 192.168.0.1#63519 (www.google.com): query: www.google.com IN A + (192.168.0.100)
		re := regexp.MustCompile(`query: ([^\s]+)`)

		for line := range *sender {
			log.Debugln(line)
			match := re.FindStringSubmatch(line)
			if len(match) > 0 {
				name := match[1]

				if len(include) > 0 {
					if _, ok := include[name]; ok {
						(*stats)[match[1]]++
					}
				} else if len(exclude) > 0 {
					if _, ok := exclude[name]; !ok {
						(*stats)[match[1]]++
					}
				} else {
					(*stats)[match[1]]++
				}
			}
		}
	}(sender, &stats, &include, &exclude)

	namesMetric := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "names",
			Name:      "names",
			Help:      "Queries per DNS name",
		},
		[]string{"domain"},
	)

	totalMetric := prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "names",
			Name:      "total",
			Help:      "Sum of all queries matched. If no include/exclude filter is present, this will match bind_query_stats_total in the stats collector.  It is initialized to 0 to support increment() detection.",
		},
	)
	totalMetric.Add(0)

	scrapesTotalMetric := prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "names",
			Name:      "scrapes_total",
			Help:      "Total number of scrapes for BIND names stats.",
		},
	)

	scrapeErrorsTotalMetric := prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "names",
			Name:      "scrape_errors_total",
			Help:      "Total number of scrapes errors for BIND names stats.",
		},
	)

	lastScrapeErrorMetric := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "names",
			Name:      "last_scrape_error",
			Help:      "Whether the last scrape of BIND names stats resulted in an error (1 for error, 0 for success).",
		},
	)

	lastScrapeTimestampMetric := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "names",
			Name:      "last_scrape_timestamp",
			Help:      "Number of seconds since 1970 since last scrape of BIND names metrics.",
		},
	)

	lastScrapeDurationSecondsMetric := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "names",
			Name:      "last_scrape_duration_seconds",
			Help:      "Duration of the last scrape of BIND names stats.",
		},
	)

	return &NamesCollector{
		stats:       stats,
		namespace:   namespace,
		namesMetric: *namesMetric,
		totalMetric: totalMetric,

		scrapesTotalMetric:              scrapesTotalMetric,
		scrapeErrorsTotalMetric:         scrapeErrorsTotalMetric,
		lastScrapeErrorMetric:           lastScrapeErrorMetric,
		lastScrapeTimestampMetric:       lastScrapeTimestampMetric,
		lastScrapeDurationSecondsMetric: lastScrapeDurationSecondsMetric,
	}
}

func makeList(fileName string, result *map[string]bool) error {
	file, err := os.Open(fileName)
	if err != nil {
		return err
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		log.Debugln("  ", scanner.Text())
		(*result)[scanner.Text()] = true
	}

	if err:= scanner.Err(); err != nil {
		return err
	}

	return nil
}

func (c *NamesCollector) Collect(ch chan<- prometheus.Metric) {
	var begun = time.Now()

	errorMetric := float64(0)
	for k, v := range c.stats {
		c.totalMetric.Add(v)
		c.namesMetric.WithLabelValues(k).Add(v)
		delete(c.stats, k)
	}
	c.totalMetric.Collect(ch)
	c.namesMetric.Collect(ch)

	c.scrapeErrorsTotalMetric.Collect(ch)

	c.scrapesTotalMetric.Inc()
	c.scrapesTotalMetric.Collect(ch)

	c.lastScrapeErrorMetric.Set(errorMetric)
	c.lastScrapeErrorMetric.Collect(ch)

	c.lastScrapeTimestampMetric.Set(float64(time.Now().Unix()))
	c.lastScrapeTimestampMetric.Collect(ch)

	c.lastScrapeDurationSecondsMetric.Set(time.Since(begun).Seconds())
	c.lastScrapeDurationSecondsMetric.Collect(ch)
}

func (c *NamesCollector) Describe(ch chan<- *prometheus.Desc) {
	c.namesMetric.Describe(ch)
	c.totalMetric.Describe(ch)
	c.scrapesTotalMetric.Describe(ch)
	c.scrapeErrorsTotalMetric.Describe(ch)
	c.lastScrapeErrorMetric.Describe(ch)
	c.lastScrapeTimestampMetric.Describe(ch)
	c.lastScrapeDurationSecondsMetric.Describe(ch)
}