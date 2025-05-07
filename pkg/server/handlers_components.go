package server

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"sigs.k8s.io/yaml"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	pkgcustomplugins "github.com/leptonai/gpud/pkg/custom-plugins"
	"github.com/leptonai/gpud/pkg/errdefs"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const (
	RequestHeaderContentType    = "Content-Type"
	RequestHeaderYAML           = "application/yaml"
	RequestHeaderJSON           = "application/json"
	RequestHeaderJSONIndent     = "json-indent"
	RequestHeaderAcceptEncoding = "Accept-Encoding"
	RequestHeaderEncodingGzip   = "gzip"
)

func (g *globalHandler) registerComponentRoutes(r gin.IRoutes) {
	r.GET(URLPathComponents, g.getComponents)
	r.GET(URLPathComponentsTriggerCheck, g.triggerComponentCheck)
	r.GET(URLPathComponentsCustomPlugins, g.getComponentsCustomPlugins)
	r.GET(URLPathComponentsTriggerTag, g.triggerComponentsByTag)

	if g.cfg.EnablePluginAPI {
		r.DELETE(URLPathComponents, g.deregisterComponent)

		r.POST(URLPathComponentsCustomPlugins, g.registerComponentsCustomPlugin)
		r.PUT(URLPathComponentsCustomPlugins, g.updateComponentsCustomPlugin)
	}

	r.GET(URLPathStates, g.getHealthStates)
	r.GET(URLPathEvents, g.getEvents)
	r.GET(URLPathInfo, g.getInfo)
	r.GET(URLPathMetrics, g.getMetrics)
}

// URLPathComponents is for getting the list of all gpud components
const URLPathComponents = "/components"

// URLPathComponentsTriggerTag is for triggering all components that have the specified tag
const URLPathComponentsTriggerTag = "/components/trigger-tag"

// getComponents godoc
// @Summary Fetch all components in gpud
// @Description get gpud components
// @ID getComponents
// @Produce  json
// @Success 200 {object} []string
// @Router /v1/components [get]
func (g *globalHandler) getComponents(c *gin.Context) {
	components := make([]string, 0)
	for _, c := range g.componentsRegistry.All() {
		components = append(components, c.Name())
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

// deregisterComponent godoc
// @Summary Deregisters a component in gpud
// @Description deregister a component in gpud
// @ID deregisterComponent
// @Produce  json
// @Success 200 {object}
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
// @Summary Manually trigger a component check in gpud
// @Description Manually trigger a component check in gpud
// @ID triggerComponentCheck
// @Produce  json
// @Success 200 {object}
// @Router /v1/components/trigger-check [get]
func (g *globalHandler) triggerComponentCheck(c *gin.Context) {
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

	rs := comp.Check()
	c.JSON(http.StatusOK, rs.HealthStates())
}

const URLPathComponentsCustomPlugins = "/components/custom-plugin"

// getComponentsCustomPlugins godoc
// @Summary Lists all custom plugins in gpud
// @Description list all custom plugins in gpud
// @ID getComponentsCustomPlugins
// @Produce  json
// @Success 200 {object} map[string]pkgcustomplugins.Spec
// @Router /v1/components/custom-plugin [get]
func (g *globalHandler) getComponentsCustomPlugins(c *gin.Context) {
	cs := make(map[string]pkgcustomplugins.Spec, 0)
	for _, c := range g.componentsRegistry.All() {
		if customPluginRegisteree, ok := c.(pkgcustomplugins.CustomPluginRegisteree); ok {
			if customPluginRegisteree.IsCustomPlugin() {
				cs[c.Name()] = customPluginRegisteree.Spec()
			}
		}
	}

	switch c.GetHeader(RequestHeaderContentType) {
	case RequestHeaderYAML:
		yb, err := yaml.Marshal(cs)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": http.StatusInternalServerError, "message": "failed to marshal custom plugins " + err.Error()})
			return
		}
		c.String(http.StatusOK, string(yb))

	case RequestHeaderJSON, "":
		if c.GetHeader(RequestHeaderJSONIndent) == "true" {
			c.IndentedJSON(http.StatusOK, cs)
			return
		}
		c.JSON(http.StatusOK, cs)

	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "invalid content type"})
	}
}

// registerComponentsCustomPlugin godoc
// @Summary Registers a new component in gpud
// @Description register a new component in gpud
// @ID registerComponentsCustomPlugin
// @Produce  json
// @Success 200 {object}
// @Router /v1/components [post]
func (g *globalHandler) registerComponentsCustomPlugin(c *gin.Context) {
	var spec pkgcustomplugins.Spec
	if err := c.BindJSON(&spec); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "failed to parse components: " + err.Error()})
		return
	}

	if err := spec.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "failed to validate custom plugin: " + err.Error()})
		return
	}

	initFunc := spec.NewInitFunc()
	if initFunc == nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "failed to create init function"})
		return
	}

	comp, err := g.componentsRegistry.Register(initFunc)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": errdefs.ErrUnknown, "message": "failed to register component: " + err.Error()})
		return
	}

	if err := comp.Start(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": errdefs.ErrUnknown, "message": "failed to start component: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": http.StatusOK, "message": "component registered and started", "component": comp.Name()})
}

// updateComponentsCustomPlugin godoc
// @Summary Registers a new component in gpud
// @Description register a new component in gpud
// @ID updateComponentsCustomPlugin
// @Produce  json
// @Success 200 {object}
// @Router /v1/components [put]
func (g *globalHandler) updateComponentsCustomPlugin(c *gin.Context) {
	var spec pkgcustomplugins.Spec
	if err := c.BindJSON(&spec); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "failed to parse components: " + err.Error()})
		return
	}

	if err := spec.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "failed to validate custom plugin: " + err.Error()})
		return
	}

	initFunc := spec.NewInitFunc()
	if initFunc == nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "failed to create init function"})
		return
	}

	prevComp := g.componentsRegistry.Get(spec.ComponentName())
	if prevComp == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": errdefs.ErrNotFound, "message": "component not found"})
		return
	}

	// now that we know the component is registered, we can deregister and register it
	prevComp = g.componentsRegistry.Deregister(prevComp.Name())
	_ = prevComp.Close()

	comp, err := g.componentsRegistry.Register(initFunc)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": errdefs.ErrUnknown, "message": "failed to register component: " + err.Error()})
		return
	}

	if err := comp.Start(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "failed to start component: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": http.StatusOK, "message": "component updated and started", "component": comp.Name()})
}

// URLPathStates is for getting the states of all gpud components
const URLPathStates = "/states"

// getHealthStates godoc
// @Summary Query component States interface in gpud
// @Description get component States interface by component name
// @ID getHealthStates
// @Param   component     query    string     false        "Component Name, leave empty to query all components"
// @Produce  json
// @Success 200 {object} v1.LeptonStates
// @Router /v1/states [get]
func (g *globalHandler) getHealthStates(c *gin.Context) {
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
		component := g.componentsRegistry.Get(componentName)
		if component == nil {
			log.Logger.Errorw("failed to get component",
				"operation", "GetStates",
				"component", componentName,
				"error", errdefs.ErrNotFound,
			)
			states = append(states, currState)
			continue
		}

		log.Logger.Debugw("getting states", "component", componentName)
		state := component.LastHealthStates()

		log.Logger.Debugw("successfully got states", "component", componentName)
		currState.States = state

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

// URLPathEvents is for getting the events of all gpud components
const URLPathEvents = "/events"

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
		component := g.componentsRegistry.Get(componentName)
		if component == nil {
			log.Logger.Errorw("failed to get component",
				"operation", "GetEvents",
				"component", componentName,
				"error", errdefs.ErrNotFound,
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

// URLPathInfo is for getting the information of all gpud components
const URLPathInfo = "/info"

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
		component := g.componentsRegistry.Get(componentName)
		if component == nil {
			log.Logger.Errorw("failed to get component",
				"operation", "GetInfo",
				"component", componentName,
				"error", errdefs.ErrNotFound,
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

		state := component.LastHealthStates()
		currInfo.Info.States = state

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

// URLPathMetrics is for getting the metrics of all gpud components
const URLPathMetrics = "/metrics"

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

// triggerComponentsByTag triggers all components that have the specified tag
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
	triggeredCount := 0

	for _, comp := range components {
		// Check if component has the specified tag
		// For now, we'll do a linear scan through all components
		// This could be optimized with a tag-based index structure
		if spec, ok := comp.(pkgcustomplugins.CustomPluginRegisteree); ok {
			hasTag := false
			for _, tag := range spec.Spec().Tags {
				if tag == tagName {
					hasTag = true
					break
				}
			}
			if hasTag {
				triggeredCount++
				if err := comp.Check(); err != nil {
					success = false
				}
			}
		}
	}

	if triggeredCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("No components found with tag: %s", tagName),
		})
		return
	}

	exitStatus := "all tests exited with status 0"
	if !success {
		exitStatus = "not all tests exited with status 0"
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    success,
		"message":    fmt.Sprintf("Triggered %d components with tag: %s", triggeredCount, tagName),
		"exitStatus": exitStatus,
	})
}
