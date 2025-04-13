package server

import (
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"sigs.k8s.io/yaml"

	apiv1 "github.com/leptonai/gpud/api/v1"
	gpudcomponents "github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/errdefs"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const (
	RequestHeaderContentType = "Content-Type"
	RequestHeaderJSON        = "application/json"
	RequestHeaderYAML        = "application/yaml"
	RequestHeaderJSONIndent  = "json-indent"

	RequestHeaderAcceptEncoding = "Accept-Encoding"
	RequestHeaderEncodingGzip   = "gzip"
)

func (g *globalHandler) registerComponentRoutes(r gin.IRoutes) {
	r.GET(URLPathComponents, g.getComponents)
	r.GET(URLPathStates, g.getStates)
	r.GET(URLPathEvents, g.getEvents)
	r.GET(URLPathInfo, g.getInfo)
	r.GET(URLPathMetrics, g.getMetrics)
}

const (
	URLPathComponents     = "/components"
	URLPathComponentsDesc = "Get the list of all components"
)

// getComponents godoc
// @Summary Fetch all components in gpud
// @Description get gpud components
// @ID getComponents
// @Produce  json
// @Success 200 {object} []string
// @Router /v1/components [get]
func (g *globalHandler) getComponents(c *gin.Context) {
	components := make([]string, 0, len(g.components))
	for name := range g.components {
		components = append(components, name)
	}
	sort.Strings(components)

	switch c.GetHeader(RequestHeaderContentType) {
	case RequestHeaderYAML:
		yb, err := yaml.Marshal(components)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": http.StatusInternalServerError, "message": "failed to marshal components " + err.Error()})
			return
		}
		c.String(http.StatusOK, string(yb))

	case RequestHeaderJSON, "":
		if c.GetHeader(RequestHeaderJSONIndent) == "true" {
			c.IndentedJSON(http.StatusOK, components)
			return
		}
		c.JSON(http.StatusOK, components)

	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "invalid content type"})
	}
}

const (
	URLPathStates     = "/states"
	URLPathStatesDesc = "Get the states of all gpud components"
)

// getStates godoc
// @Summary Query component States interface in gpud
// @Description get component States interface by component name
// @ID getStates
// @Param   component     query    string     false        "Component Name, leave empty to query all components"
// @Produce  json
// @Success 200 {object} v1.LeptonStates
// @Router /v1/states [get]
func (g *globalHandler) getStates(c *gin.Context) {
	var states apiv1.GPUdComponentHealthStates
	components, err := g.getReqComponents(c)
	if err != nil {
		if errdefs.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"code": errdefs.ErrNotFound, "message": "component not found: " + err.Error()})
			return
		}

		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "failed to parse components: " + err.Error()})
		return
	}
	for _, componentName := range components {
		currState := apiv1.ComponentHealthStates{
			Component: componentName,
		}
		component, err := gpudcomponents.GetComponent(componentName)
		if err != nil {
			log.Logger.Errorw("failed to get component",
				"operation", "GetStates",
				"component", componentName,
				"error", err,
			)
			states = append(states, currState)
			continue
		}

		log.Logger.Debugw("getting states", "component", componentName)
		state, err := component.HealthStates(c)
		if err != nil {
			log.Logger.Errorw("failed to invoke component state",
				"operation", "GetStates",
				"component", componentName,
				"error", err,
			)
		} else {
			log.Logger.Debugw("successfully got states", "component", componentName)
			currState.States = state
		}
		states = append(states, currState)
	}

	switch c.GetHeader(RequestHeaderContentType) {
	case RequestHeaderYAML:
		yb, err := yaml.Marshal(states)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": http.StatusInternalServerError, "message": "failed to marshal states " + err.Error()})
			return
		}
		c.String(http.StatusOK, string(yb))

	case RequestHeaderJSON, "":
		if c.GetHeader(RequestHeaderJSONIndent) == "true" {
			c.IndentedJSON(http.StatusOK, states)
			return
		}
		c.JSON(http.StatusOK, states)

	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "invalid content type"})
	}
}

const (
	URLPathEvents     = "/events"
	URLPathEventsDesc = "Get the events of all gpud components"
)

// getEvents godoc
// @Summary Query component Events interface in gpud
// @Description get component Events interface by component name
// @ID getEvents
// @Param   component     query    string     false        "Component Name, leave empty to query all components"
// @Produce  json
// @Success 200 {object} v1.LeptonEvents
// @Router /v1/events [get]
func (g *globalHandler) getEvents(c *gin.Context) {
	var events apiv1.GPUdComponentEvents
	components, err := g.getReqComponents(c)
	if err != nil {
		if errdefs.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"code": errdefs.ErrNotFound, "message": "component not found: " + err.Error()})
			return
		}

		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "failed to parse components: " + err.Error()})
		return
	}
	startTime, endTime, err := g.getReqTime(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "failed to parse time: " + err.Error()})
		return
	}
	for _, componentName := range components {
		currEvent := apiv1.ComponentEvents{
			Component: componentName,
			StartTime: startTime,
			EndTime:   endTime,
		}
		component, err := gpudcomponents.GetComponent(componentName)
		if err != nil {
			log.Logger.Errorw("failed to get component",
				"operation", "GetEvents",
				"component", componentName,
				"error", err,
			)
			events = append(events, currEvent)
			continue
		}
		event, err := component.Events(c, startTime)
		if err != nil {
			log.Logger.Errorw("failed to invoke component events",
				"operation", "GetEvents",
				"component", componentName,
				"error", err,
			)
		} else if len(event) > 0 {
			currEvent.Events = event
		}
		events = append(events, currEvent)
	}

	switch c.GetHeader(RequestHeaderContentType) {
	case RequestHeaderYAML:
		yb, err := yaml.Marshal(events)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": http.StatusInternalServerError, "message": "failed to marshal events " + err.Error()})
			return
		}
		c.String(http.StatusOK, string(yb))

	case RequestHeaderJSON, "":
		if c.GetHeader(RequestHeaderJSONIndent) == "true" {
			c.IndentedJSON(http.StatusOK, events)
			return
		}
		c.JSON(http.StatusOK, events)

	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "invalid content type"})
	}
}

const DefaultQuerySince = 30 * time.Minute

const (
	URLPathInfo     = "/info"
	URLPathInfoDesc = "Get the information of all gpud components"
)

// getInfo godoc
// @Summary Query component Events/Metrics/States interface in gpud
// @Description get component Events/Metrics/States interface by component name
// @ID getInfo
// @Param   component     query    string     false        "Component Name, leave empty to query all components"
// @Produce  json
// @Success 200 {object} v1.LeptonInfo
// @Router /v1/info [get]
func (g *globalHandler) getInfo(c *gin.Context) {
	var infos apiv1.GPUdComponentInfos
	reqComps, err := g.getReqComponents(c)
	if err != nil {
		if errdefs.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"code": errdefs.ErrNotFound, "message": "component not found: " + err.Error()})
			return
		}

		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "failed to parse components: " + err.Error()})
		return
	}
	startTime, endTime, err := g.getReqTime(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "failed to parse time: " + err.Error()})
		return
	}

	now := startTime.UTC()
	metricsSince := now.Add(-DefaultQuerySince)
	if sinceRaw := c.Query("since"); sinceRaw != "" {
		dur, err := time.ParseDuration(sinceRaw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "failed to parse duration: " + err.Error()})
			return
		}
		metricsSince = now.Add(-dur)
	}

	metricsData, err := g.metricsStore.Read(c, pkgmetrics.WithSince(metricsSince), pkgmetrics.WithComponents(reqComps...))
	if err != nil {
		log.Logger.Errorw("failed to invoke component metrics",
			"operation", "GetInfo",
			"components", reqComps,
			"error", err,
		)
	}

	componentsToMetrics := make(map[string][]apiv1.Metric)
	for _, data := range metricsData {
		if _, ok := componentsToMetrics[data.Component]; !ok {
			componentsToMetrics[data.Component] = make([]apiv1.Metric, 0)
		}
		d := apiv1.Metric{
			UnixSeconds:                   data.UnixMilliseconds,
			DeprecatedMetricName:          data.Name,
			DeprecatedMetricSecondaryName: data.Label,
			Value:                         data.Value,
		}
		componentsToMetrics[data.Component] = append(componentsToMetrics[data.Component], d)
	}

	for _, componentName := range reqComps {
		currInfo := apiv1.ComponentInfo{
			Component: componentName,
			StartTime: startTime,
			EndTime:   endTime,
			Info:      apiv1.Info{},
		}
		component, err := gpudcomponents.GetComponent(componentName)
		if err != nil {
			log.Logger.Errorw("failed to get component",
				"operation", "GetInfo",
				"component", componentName,
				"error", err,
			)
			infos = append(infos, currInfo)
			continue
		}
		events, err := component.Events(c, startTime)
		if err != nil {
			log.Logger.Errorw("failed to invoke component events",
				"operation", "GetInfo",
				"component", componentName,
				"error", err,
			)
		} else if len(events) > 0 {
			currInfo.Info.Events = events
		}
		state, err := component.HealthStates(c)
		if err != nil {
			log.Logger.Errorw("failed to invoke component states",
				"operation", "GetInfo",
				"component", componentName,
				"error", err,
			)
		} else {
			currInfo.Info.States = state
		}

		currInfo.Info.Metrics = componentsToMetrics[componentName]

		infos = append(infos, currInfo)
	}

	switch c.GetHeader(RequestHeaderContentType) {
	case RequestHeaderYAML:
		yb, err := yaml.Marshal(infos)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": http.StatusInternalServerError, "message": "failed to marshal infos " + err.Error()})
			return
		}
		c.String(http.StatusOK, string(yb))

	case RequestHeaderJSON, "":
		if c.GetHeader(RequestHeaderJSONIndent) == "true" {
			c.IndentedJSON(http.StatusOK, infos)
			return
		}
		c.JSON(http.StatusOK, infos)

	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "invalid content type"})
	}
}

const (
	URLPathMetrics     = "/metrics"
	URLPathMetricsDesc = "Get the metrics of all gpud components"
)

// getMetrics godoc
// @Summary Query component Metrics interface in gpud
// @Description get component Metrics interface by component name
// @ID getMetrics
// @Param   component     query    string     false        "Component Name, leave empty to query all components"
// @Produce  json
// @Success 200 {object} v1.LeptonMetrics
// @Router /v1/metrics [get]
func (g *globalHandler) getMetrics(c *gin.Context) {
	components, err := g.getReqComponents(c)
	if err != nil {
		if errdefs.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"code": errdefs.ErrNotFound, "message": "component not found: " + err.Error()})
			return
		}

		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "failed to parse components: " + err.Error()})
		return
	}

	now := time.Now().UTC()
	metricsSince := now.Add(-DefaultQuerySince)
	if sinceRaw := c.Query("since"); sinceRaw != "" {
		dur, err := time.ParseDuration(sinceRaw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "failed to parse duration: " + err.Error()})
			return
		}
		metricsSince = now.Add(-dur)
	}

	metricsData, err := g.metricsStore.Read(c, pkgmetrics.WithSince(metricsSince), pkgmetrics.WithComponents(components...))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": http.StatusInternalServerError, "message": "failed to read metrics: " + err.Error()})
		return
	}

	metrics := pkgmetrics.ConvertToLeptonMetrics(metricsData)
	switch c.GetHeader(RequestHeaderContentType) {
	case RequestHeaderYAML:
		yb, err := yaml.Marshal(metrics)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": http.StatusInternalServerError, "message": "failed to marshal metrics " + err.Error()})
			return
		}
		c.String(http.StatusOK, string(yb))

	case RequestHeaderJSON, "":
		if c.GetHeader(RequestHeaderJSONIndent) == "true" {
			c.IndentedJSON(http.StatusOK, metrics)
			return
		}
		c.JSON(http.StatusOK, metrics)

	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "invalid content type"})
	}
}
