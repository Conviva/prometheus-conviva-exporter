# Conviva Experience Insights Prometheus Exporter

This is a server that fetches Conviva Experience Insights metrics from the [Conviva Quality Metriclens API](https://developer.conviva.com/docs/eiapi/) and formats it for Prometheus.

### Scrape Interval
The recommended scrape interval is 60s.

### Configuration

ENV Variable | Description | Example
----- | ----- | -----
CONVIVA_BASE_URL | Conviva API domain | https://api.conviva.com
CONVIVA_API_VERSION | Insights API version | 2.6
CONVIVA_CLIENT_ID | API Client ID. You can get your Conviva Client ID and Secret at: https://pulse.conviva.com/app/admin/apps/list
CONVIVA_CLIENT_SECRET | API Client Secret
CONVIVA_FILTER_IDS | Comma-separated list of filter IDs. Filters can be created in Pulse and Filter IDs can be obtained from /insights/2.6/filters.json | 1234,5678
CONVIVA_DIMENSION_ID | Single dimension ID. Available dimension IDs can be obtained from /insights/2.6/metriclens_dimension_list.json | 12345

### Docker
Build container
```
docker build -t conviva-prometheus-exporter .
```

Run
```
docker run --env-file ./.env -dp 8080:8080 conviva-prometheus-exporter
```

### Limitations
Ad metrics are currently not available
