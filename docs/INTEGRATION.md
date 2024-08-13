# Integration with GPUd

GPUd is a powerful service designed to collect node metrics and expose them via a secure HTTPS API, running on port 15132 by default. It provides users with the ability to gather and analyze node data, making it easy to interact with through a customizable client.

## Key Features

* Collects metrics, states, and events from nodes.
* Provides a simple RESTful API to access collected data.
* Supports secure access via HTTPS.

## API Overview

GPUd provides the following primary API endpoints:

    GET /v1/components: Retrieve a list of all components in GPUd.
    GET /v1/events: Query component events by component name. If no name is specified, events for all components are returned.
    GET /v1/info: Retrieve events, metrics, and states for a specific component. If no name is specified, data for all components is returned.
    GET /v1/metrics: Query metrics for a specific component. If no name is specified, metrics for all components are returned.
    GET /v1/states: Query states for a specific component. If no name is specified, states for all components are returned.

For detailed documentation, visit the [GPUd API Documentation](https://gpud.ai/api/v1/docs).

## Integration Steps
1.	Install and Start GPUd: Follow the instructions in the [Get Started](../README.md#get-started) guide.
2.	Access the API: Use a client to interact with the GPUd API. You can find a [sample client](../examples/client/main.go) in the examples directory.
3.	Import GPUd Client: For deeper integration, import the provided [Client](../client) set into your project.