package server

import (
	"errors"
	"net/http"
	"sort"
	"time"

	v1 "github.com/leptonai/gpud/api/v1"
	lep_components "github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/errdefs"
	"github.com/leptonai/gpud/log"

	"github.com/gin-gonic/gin"
	"sigs.k8s.io/yaml"
)

const (
	RequestHeaderContentType = "Content-Type"
	RequestHeaderJSON        = "application/json"
	RequestHeaderYAML        = "application/yaml"
	RequestHeaderJSONIndent  = "json-indent"

	RequestHeaderAcceptEncoding = "Accept-Encoding"
	RequestHeaderEncodingGzip   = "gzip"
)

type componentHandlerDescription struct {
	Path string
	Desc string
}

func (g *globalHandler) registerComponentRoutes(r gin.IRoutes) []componentHandlerDescription {
	paths := make([]componentHandlerDescription, 0)

	r.GET(URLPathComponents, g.getComponents)
	paths = append(paths, componentHandlerDescription{
		Path: URLPathComponents,
		Desc: URLPathComponentsDesc,
	})

	r.GET(URLPathStates, g.getStates)
	paths = append(paths, componentHandlerDescription{
		Path: URLPathStates,
		Desc: URLPathStatesDesc,
	})

	r.GET(URLPathEvents, g.getEvents)
	paths = append(paths, componentHandlerDescription{
		Path: URLPathEvents,
		Desc: URLPathEventsDesc,
	})

	r.GET(URLPathInfo, g.getInfo)
	paths = append(paths, componentHandlerDescription{
		Path: URLPathInfo,
		Desc: URLPathInfoDesc,
	})

	r.GET(URLPathMetrics, g.getMetrics)
	paths = append(paths, componentHandlerDescription{
		Path: URLPathMetrics,
		Desc: URLPathMetricsDesc,
	})

	return paths
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
	var states v1.LeptonStates
	components, err := g.getReqComponents(c)
	if err != nil {
		if errors.Is(err, errdefs.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": errdefs.ErrNotFound, "message": "component not found: " + err.Error()})
			return
		}

		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "failed to parse components: " + err.Error()})
		return
	}
	for _, componentName := range components {
		currState := v1.LeptonComponentStates{
			Component: componentName,
		}
		component, err := lep_components.GetComponent(componentName)
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
		state, err := component.States(c)
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
	var events v1.LeptonEvents
	components, err := g.getReqComponents(c)
	if err != nil {
		if errors.Is(err, errdefs.ErrNotFound) {
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
		currEvent := v1.LeptonComponentEvents{
			Component: componentName,
			StartTime: startTime,
			EndTime:   endTime,
		}
		component, err := lep_components.GetComponent(componentName)
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
			if errors.Is(err, query.ErrNoData) {
				log.Logger.Debugw("no events found", "component", componentName)
				continue
			}

			log.Logger.Errorw("failed to invoke component events",
				"operation", "GetEvents",
				"component", componentName,
				"error", err,
			)
		} else {
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
	var infos v1.LeptonInfo
	components, err := g.getReqComponents(c)
	if err != nil {
		if errors.Is(err, errdefs.ErrNotFound) {
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

	for _, componentName := range components {
		currInfo := v1.LeptonComponentInfo{
			Component: componentName,
			StartTime: startTime,
			EndTime:   endTime,
			Info:      lep_components.Info{},
		}
		component, err := lep_components.GetComponent(componentName)
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
			if errors.Is(err, query.ErrNoData) {
				log.Logger.Debugw("no events found", "component", componentName)
				continue
			}

			log.Logger.Errorw("failed to invoke component events",
				"operation", "GetInfo",
				"component", componentName,
				"error", err,
			)
		} else {
			currInfo.Info.Events = events
		}
		state, err := component.States(c)
		if err != nil {
			log.Logger.Errorw("failed to invoke component states",
				"operation", "GetInfo",
				"component", componentName,
				"error", err,
			)
		} else {
			currInfo.Info.States = state
		}
		metric, err := component.Metrics(c, metricsSince)
		if err != nil {
			log.Logger.Errorw("failed to invoke component metrics",
				"operation", "GetInfo",
				"component", componentName,
				"error", err,
			)
		} else {
			currInfo.Info.Metrics = metric
		}
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
		if errors.Is(err, errdefs.ErrNotFound) {
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

	var metrics v1.LeptonMetrics
	for _, componentName := range components {
		currMetrics := v1.LeptonComponentMetrics{
			Component: componentName,
		}
		component, err := lep_components.GetComponent(componentName)
		if err != nil {
			log.Logger.Errorw("failed to get component",
				"operation", "GetEvents",
				"component", componentName,
				"error", err,
			)
			metrics = append(metrics, currMetrics)
			continue
		}
		currMetric, err := component.Metrics(c, metricsSince)
		if err != nil {
			log.Logger.Errorw("failed to invoke component metrics",
				"operation", "GetEvents",
				"component", componentName,
				"error", err,
			)
		} else {
			currMetrics.Metrics = currMetric
		}
		metrics = append(metrics, currMetrics)
	}

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
