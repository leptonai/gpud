package server

import (
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"sigs.k8s.io/yaml"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/errdefs"
	"github.com/leptonai/gpud/pkg/httputil"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

func (g *globalHandler) registerComponentRoutes(r gin.IRoutes) {
	r.GET(URLPathComponents, g.getComponents)
	r.DELETE(URLPathComponents, g.deregisterComponent)

	r.GET(URLPathComponentsTriggerCheck, g.triggerComponentCheck)
	r.GET(URLPathComponentsTriggerTag, g.triggerComponentsByTag)

	r.GET(URLPathStates, g.getHealthStates)
	r.GET(URLPathEvents, g.getEvents)
	r.GET(URLPathInfo, g.getInfo)
	r.GET(URLPathMetrics, g.getMetrics)

	r.POST(URLPathHealthStatesSetHealthy, g.setHealthyStates)
}

// URLPathComponents is for getting the list of all gpud components
const URLPathComponents = "/components"

// getComponents godoc
// @Summary Get list of registered components
// @Description Returns a list of all currently registered gpud components in the system
// @ID getComponents
// @Tags components
// @Accept json
// @Produce json
// @Header 200 {string} Content-Type "application/json or application/yaml"
// @Param Accept header string false "Content type preference" Enums(application/json,application/yaml)
// @Param json-indent header string false "Set to 'true' for indented JSON output"
// @Success 200 {array} string "List of component names"
// @Failure 400 {object} map[string]interface{} "Bad request - invalid content type"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /v1/components [get]
func (g *globalHandler) getComponents(c *gin.Context) {
	components := make([]string, 0)
	for _, c := range g.componentsRegistry.All() {
		components = append(components, c.Name())
	}
	sort.Strings(components)

	switch c.GetHeader(httputil.RequestHeaderContentType) {
	case httputil.RequestHeaderYAML:
		yb, err := yaml.Marshal(components)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": http.StatusInternalServerError, "message": "failed to marshal components " + err.Error()})
			return
		}
		c.String(http.StatusOK, string(yb))

	case httputil.RequestHeaderJSON, "":
		if c.GetHeader(httputil.RequestHeaderJSONIndent) == "true" {
			c.IndentedJSON(http.StatusOK, components)
			return
		}
		c.JSON(http.StatusOK, components)

	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "invalid content type"})
	}
}

// deregisterComponent godoc
// @Summary Deregister a component
// @Description Deregisters a component from the system if it supports deregistration. Only components that implement the Deregisterable interface can be deregistered.
// @ID deregisterComponent
// @Tags components
// @Accept json
// @Produce json
// @Param componentName query string true "Name of the component to deregister"
// @Success 200 {object} map[string]interface{} "Component deregistered successfully"
// @Failure 400 {object} map[string]interface{} "Bad request - component name required or component not deregisterable"
// @Failure 404 {object} map[string]interface{} "Component not found"
// @Failure 500 {object} map[string]interface{} "Internal server error - failed to close component"
// @Router /v1/components [delete]
func (g *globalHandler) deregisterComponent(c *gin.Context) {
	componentName := c.Query("componentName")
	if componentName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "component name is required"})
		return
	}

	comp := g.componentsRegistry.Get(componentName)
	if comp == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": errdefs.ErrNotFound, "message": "component not found"})
		return
	}

	deregisterable, ok := comp.(components.Deregisterable)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "component is not deregisterable"})
		return
	}

	if !deregisterable.CanDeregister() {
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "component is not deregisterable"})
		return
	}

	if err := comp.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": http.StatusInternalServerError, "message": "failed to deregister component: " + err.Error()})
		return
	}

	// only deregister if the component is successfully closed
	_ = g.componentsRegistry.Deregister(componentName)

	c.JSON(http.StatusOK, gin.H{"code": http.StatusOK, "message": "component deregistered", "component": comp.Name()})
}

const URLPathComponentsTriggerCheck = "/components/trigger-check"

// triggerComponentCheck godoc
// @Summary Trigger component health check
// @Description Triggers a health check for a specific component or all components with a specific tag. Either componentName or tagName must be provided, but not both.
// @ID triggerComponentCheck
// @Tags components
// @Accept json
// @Produce json
// @Param componentName query string false "Name of the specific component to check (mutually exclusive with tagName)"
// @Param tagName query string false "Tag name to check all components with this tag (mutually exclusive with componentName)"
// @Success 200 {object} apiv1.GPUdComponentHealthStates "Health check results with component states"
// @Failure 400 {object} map[string]interface{} "Bad request - component or tag name required (but not both)"
// @Failure 404 {object} map[string]interface{} "Component not found"
// @Router /v1/components/trigger-check [get]
func (g *globalHandler) triggerComponentCheck(c *gin.Context) {
	componentName := c.Query("componentName")
	tagName := c.Query("tagName")

	if componentName == "" && tagName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "component or tag name is required"})
		return
	}

	checkResults := make([]components.CheckResult, 0)
	if componentName != "" {
		// requesting a specific component, tag is ignored
		comp := g.componentsRegistry.Get(componentName)
		if comp == nil {
			c.JSON(http.StatusNotFound, gin.H{"code": errdefs.ErrNotFound, "message": "component not found"})
			return
		}

		checkResults = append(checkResults, comp.Check())
	} else if tagName != "" {
		components := g.componentsRegistry.All()
		for _, comp := range components {
			matched := false
			for _, tag := range comp.Tags() {
				if tag == tagName {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}

			checkResults = append(checkResults, comp.Check())
		}
	}

	resp := apiv1.GPUdComponentHealthStates{}
	for _, checkResult := range checkResults {
		resp = append(resp, apiv1.ComponentHealthStates{
			Component: checkResult.ComponentName(),
			States:    checkResult.HealthStates(),
		})
	}
	c.JSON(http.StatusOK, resp)
}

// URLPathComponentsTriggerTag is for triggering components by tag
const URLPathComponentsTriggerTag = "/components/trigger-tag"

// triggerComponentsByTag godoc
// @Summary Trigger components by tag
// @Description Triggers health checks for all components that have the specified tag. Returns a summary of triggered components and their overall status.
// @ID triggerComponentsByTag
// @Tags components
// @Accept json
// @Produce json
// @Param tagName query string true "Tag name to trigger all components with this tag"
// @Success 200 {object} map[string]interface{} "Trigger results with components list, exit status, and success flag"
// @Failure 400 {object} map[string]interface{} "Bad request - tag name required"
// @Router /v1/components/trigger-tag [get]
func (g *globalHandler) triggerComponentsByTag(c *gin.Context) {
	tagName := c.Query("tagName")
	if tagName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tagName parameter is required"})
		return
	}

	// TODO: Consider implementing a tag-based index structure to avoid linear scan
	// This could be a map[tag][]Component or similar structure that's maintained
	// when components are registered/deregistered
	components := g.componentsRegistry.All()
	success := true
	triggeredComponents := make([]string, 0)
	exitStatus := 0

	for _, comp := range components {
		// Check if component has the specified tag using the Tags() method
		tags := comp.Tags()
		for _, tag := range tags {
			if tag == tagName {
				triggeredComponents = append(triggeredComponents, comp.Name())
				if err := comp.Check(); err != nil {
					success = false
					exitStatus = 1
				}
				break
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"components": triggeredComponents,
		"exit":       exitStatus,
		"success":    success,
	})
}

// URLPathStates is for getting the states of all gpud components
const URLPathStates = "/states"

// getHealthStates godoc
// @Summary Get component health states
// @Description Returns the current health states of specified components or all components if none specified. Only supported components are included in the response.
// @ID getHealthStates
// @Tags components
// @Accept json
// @Produce json
// @Header 200 {string} Content-Type "application/json or application/yaml"
// @Param Accept header string false "Content type preference" Enums(application/json,application/yaml)
// @Param components query string false "Comma-separated list of component names to query (if empty, returns all components)"
// @Param json-indent header string false "Set to 'true' for indented JSON output"
// @Success 200 {object} apiv1.GPUdComponentHealthStates "Component health states"
// @Failure 400 {object} map[string]interface{} "Bad request - invalid content type or component parsing error"
// @Failure 404 {object} map[string]interface{} "Component not found"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /v1/states [get]
func (g *globalHandler) getHealthStates(c *gin.Context) {
	var states apiv1.GPUdComponentHealthStates
	components, err := g.getReqComponentNames(c)
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

		comp := g.componentsRegistry.Get(componentName)
		if comp == nil {
			log.Logger.Errorw("failed to get component",
				"operation", "GetStates",
				"component", componentName,
				"error", errdefs.ErrNotFound,
			)
			states = append(states, currState)
			continue
		}
		if !comp.IsSupported() {
			log.Logger.Debugw("component not supported", "component", componentName)
			continue
		}

		log.Logger.Debugw("getting states", "component", componentName)
		state := comp.LastHealthStates()

		log.Logger.Debugw("successfully got states", "component", componentName)
		currState.States = state

		states = append(states, currState)
	}

	switch c.GetHeader(httputil.RequestHeaderContentType) {
	case httputil.RequestHeaderYAML:
		yb, err := yaml.Marshal(states)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": http.StatusInternalServerError, "message": "failed to marshal states " + err.Error()})
			return
		}
		c.String(http.StatusOK, string(yb))

	case httputil.RequestHeaderJSON, "":
		if c.GetHeader(httputil.RequestHeaderJSONIndent) == "true" {
			c.IndentedJSON(http.StatusOK, states)
			return
		}
		c.JSON(http.StatusOK, states)

	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "invalid content type"})
	}
}

// URLPathEvents is for getting the events of all gpud components
const URLPathEvents = "/events"

// getEvents godoc
// @Summary Get component events
// @Description Returns events from specified components within a time range. If no components specified, returns events from all components. Only supported components are queried.
// @ID getEvents
// @Tags components
// @Accept json
// @Produce json
// @Header 200 {string} Content-Type "application/json or application/yaml"
// @Param Accept header string false "Content type preference" Enums(application/json,application/yaml)
// @Param components query string false "Comma-separated list of component names to query (if empty, queries all components)"
// @Param startTime query string false "Start time for event query (RFC3339 format, defaults to current time)"
// @Param endTime query string false "End time for event query (RFC3339 format, defaults to current time)"
// @Param json-indent header string false "Set to 'true' for indented JSON output"
// @Success 200 {object} apiv1.GPUdComponentEvents "Component events within the specified time range"
// @Failure 400 {object} map[string]interface{} "Bad request - invalid content type, component parsing error, or time parsing error"
// @Failure 404 {object} map[string]interface{} "Component not found"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /v1/events [get]
func (g *globalHandler) getEvents(c *gin.Context) {
	var events apiv1.GPUdComponentEvents
	components, err := g.getReqComponentNames(c)
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

		comp := g.componentsRegistry.Get(componentName)
		if comp == nil {
			log.Logger.Errorw("failed to get component",
				"operation", "GetEvents",
				"component", componentName,
				"error", errdefs.ErrNotFound,
			)
			events = append(events, currEvent)
			continue
		}
		if !comp.IsSupported() {
			log.Logger.Debugw("component not supported", "component", componentName)
			continue
		}

		event, err := comp.Events(c, startTime)
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

	switch c.GetHeader(httputil.RequestHeaderContentType) {
	case httputil.RequestHeaderYAML:
		yb, err := yaml.Marshal(events)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": http.StatusInternalServerError, "message": "failed to marshal events " + err.Error()})
			return
		}
		c.String(http.StatusOK, string(yb))

	case httputil.RequestHeaderJSON, "":
		if c.GetHeader(httputil.RequestHeaderJSONIndent) == "true" {
			c.IndentedJSON(http.StatusOK, events)
			return
		}
		c.JSON(http.StatusOK, events)

	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "invalid content type"})
	}
}

const DefaultQuerySince = 30 * time.Minute

// URLPathInfo is for getting the information of all gpud components
const URLPathInfo = "/info"

// getInfo godoc
// @Summary Get comprehensive component information
// @Description Returns comprehensive information including events, states, and metrics for specified components. If no components specified, returns information for all components. Only supported components are included.
// @ID getInfo
// @Tags components
// @Accept json
// @Produce json
// @Header 200 {string} Content-Type "application/json or application/yaml"
// @Param Accept header string false "Content type preference" Enums(application/json,application/yaml)
// @Param components query string false "Comma-separated list of component names to query (if empty, queries all components)"
// @Param startTime query string false "Start time for query (RFC3339 format, defaults to current time)"
// @Param endTime query string false "End time for query (RFC3339 format, defaults to current time)"
// @Param since query string false "Duration string for metrics query (e.g., '30m', '1h') - defaults to 30 minutes"
// @Param json-indent header string false "Set to 'true' for indented JSON output"
// @Success 200 {object} apiv1.GPUdComponentInfos "Component information including events, states, and metrics"
// @Failure 400 {object} map[string]interface{} "Bad request - invalid content type, component parsing error, time parsing error, or duration parsing error"
// @Failure 404 {object} map[string]interface{} "Component not found"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /v1/info [get]
func (g *globalHandler) getInfo(c *gin.Context) {
	var infos apiv1.GPUdComponentInfos
	reqComps, err := g.getReqComponentNames(c)
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
			UnixSeconds: data.UnixMilliseconds,
			Name:        data.Name,
			Labels:      data.Labels,
			Value:       data.Value,
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

		comp := g.componentsRegistry.Get(componentName)
		if comp == nil {
			log.Logger.Errorw("failed to get component",
				"operation", "GetInfo",
				"component", componentName,
				"error", errdefs.ErrNotFound,
			)
			infos = append(infos, currInfo)
			continue
		}
		if !comp.IsSupported() {
			log.Logger.Debugw("component not supported", "component", componentName)
			continue
		}

		events, err := comp.Events(c, startTime)
		if err != nil {
			log.Logger.Errorw("failed to invoke component events",
				"operation", "GetInfo",
				"component", componentName,
				"error", err,
			)
		} else if len(events) > 0 {
			currInfo.Info.Events = events
		}

		state := comp.LastHealthStates()
		currInfo.Info.States = state

		currInfo.Info.Metrics = componentsToMetrics[componentName]

		infos = append(infos, currInfo)
	}

	switch c.GetHeader(httputil.RequestHeaderContentType) {
	case httputil.RequestHeaderYAML:
		yb, err := yaml.Marshal(infos)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": http.StatusInternalServerError, "message": "failed to marshal infos " + err.Error()})
			return
		}
		c.String(http.StatusOK, string(yb))

	case httputil.RequestHeaderJSON, "":
		if c.GetHeader(httputil.RequestHeaderJSONIndent) == "true" {
			c.IndentedJSON(http.StatusOK, infos)
			return
		}
		c.JSON(http.StatusOK, infos)

	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "invalid content type"})
	}
}

// URLPathMetrics is for getting the metrics of all gpud components
const URLPathMetrics = "/metrics"

// getMetrics godoc
// @Summary Get component metrics
// @Description Returns metrics data for specified components within a time range. If no components specified, returns metrics for all components. Metrics are queried from the last 30 minutes by default.
// @ID getMetrics
// @Tags components
// @Accept json
// @Produce json
// @Header 200 {string} Content-Type "application/json or application/yaml"
// @Param Accept header string false "Content type preference" Enums(application/json,application/yaml)
// @Param components query string false "Comma-separated list of component names to query (if empty, queries all components)"
// @Param since query string false "Duration string for metrics query (e.g., '30m', '1h') - defaults to 30 minutes"
// @Param json-indent header string false "Set to 'true' for indented JSON output"
// @Success 200 {object} apiv1.GPUdComponentMetrics "Component metrics data within the specified time range"
// @Failure 400 {object} map[string]interface{} "Bad request - invalid content type, component parsing error, or duration parsing error"
// @Failure 404 {object} map[string]interface{} "Component not found"
// @Failure 500 {object} map[string]interface{} "Internal server error - failed to read metrics"
// @Router /v1/metrics [get]
func (g *globalHandler) getMetrics(c *gin.Context) {
	components, err := g.getReqComponentNames(c)
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
	switch c.GetHeader(httputil.RequestHeaderContentType) {
	case httputil.RequestHeaderYAML:
		yb, err := yaml.Marshal(metrics)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": http.StatusInternalServerError, "message": "failed to marshal metrics " + err.Error()})
			return
		}
		c.String(http.StatusOK, string(yb))

	case httputil.RequestHeaderJSON, "":
		if c.GetHeader(httputil.RequestHeaderJSONIndent) == "true" {
			c.IndentedJSON(http.StatusOK, metrics)
			return
		}
		c.JSON(http.StatusOK, metrics)

	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "invalid content type"})
	}
}

// URLPathHealthStatesSetHealthy is for setting components to healthy state
const URLPathHealthStatesSetHealthy = "/health-states/set-healthy"

// setHealthyStates godoc
// @Summary Set components to healthy state
// @Description Sets specified components to healthy state if they implement the HealthSettable interface. If no components specified, attempts to set all components to healthy.
// @ID setHealthyStates
// @Tags components
// @Accept json
// @Produce json
// @Param components query string false "Comma-separated list of component names to set healthy (if empty, sets all components)"
// @Success 200 {object} map[string]interface{} "Components successfully set to healthy state"
// @Failure 400 {object} map[string]interface{} "Bad request - component does not support setting healthy state"
// @Failure 404 {object} map[string]interface{} "Component not found"
// @Failure 500 {object} map[string]interface{} "Internal server error - failed to set healthy state"
// @Router /v1/health-states/set-healthy [post]
func (g *globalHandler) setHealthyStates(c *gin.Context) {
	// Check if components query parameter is empty
	componentsQuery := c.Query("components")
	if componentsQuery == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    errdefs.ErrInvalidArgument,
			"message": "components parameter is required",
		})
		return
	}

	comps, err := g.getReqComponents(c)
	if err != nil {
		if errdefs.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"code": errdefs.ErrNotFound, "message": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "failed to parse components: " + err.Error()})
		return
	}

	successfulComponents := make([]string, 0)
	failedComponents := make(map[string]string)

	for _, comp := range comps {
		healthSettable, ok := comp.(components.HealthSettable)
		if !ok {
			failedComponents[comp.Name()] = "component does not support setting healthy state"
			continue
		}

		if err := healthSettable.SetHealthy(); err != nil {
			failedComponents[comp.Name()] = "failed to set healthy: " + err.Error()
			log.Logger.Errorw("failed to set component healthy",
				"component", comp.Name(),
				"error", err,
			)
		} else {
			successfulComponents = append(successfulComponents, comp.Name())
			log.Logger.Infow("successfully set component healthy", "component", comp.Name())
		}
	}

	if len(failedComponents) > 0 && len(successfulComponents) == 0 {
		// All components failed
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    errdefs.ErrInvalidArgument,
			"message": "failed to set any component to healthy",
			"failed":  failedComponents,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":       http.StatusOK,
		"message":    "set healthy states completed",
		"successful": successfulComponents,
		"failed":     failedComponents,
	})
}
