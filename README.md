# Conviva Experience Insights Prometheus Exporter

This is a server that fetches Conviva Experience Insights metrics from the [Conviva Quality Metriclens API](https://developer.conviva.com/docs/eiapi/) and formats it for Prometheus.

### Scrape Interval
The recommended scrape interval is 60s.

### Configuration

ENV Variable | Description | Example
----- | ----- | -----
CONVIVA_BASE_URL | Conviva API domain | https://api.conviva.com
CONVIVA_API_VERSION | Insights API version | 3.0
CONVIVA_CLIENT_ID | API Client ID. You can get your Conviva Client ID and Secret at: https://pulse.conviva.com/app/admin/apps/list
CONVIVA_CLIENT_SECRET | API Client Secret
CONVIVA_FILTER_ID | Filter IDs. Filters can be created in Pulse and Filter IDs can be obtained from /insights/3.0/filters.json | 1234
CONVIVA_DIMENSION_NAME | Dimension name. Available dimension IDs can be obtained from /insights/3.0/metrics/_meta/references/dimensions | player-name

### Docker
Build container
```
docker build -t conviva-prometheus-exporter .
```

Run
```
docker run --env-file ./.env -dp 8080:8080 conviva-prometheus-exporter
```

### Docker Compose
Run Prometheus and Conviva Experience Insights Prometheus Exporter
```
docker compose up -d
```
Stop
```
docker compose down
```
