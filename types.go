package main

import "github.com/aws/aws-sdk-go/aws/session"

type Statika struct {
	services []*ServiceDescription
	instanceID string
	containerInstanceID string
	configurationURL string
	session *session.Session
	region string
}

type ServiceDescription struct {
	ServiceName string `json:"serviceName"`
	LoadBalancerName string `json:"loadBalancerName"`
	ExposedContainerName string `json:"exposedContainerName"`
}

// Configuration holds the basic environment information for Statika including the name of the service description file
type Configuration struct {
	Region string `json:"region"`
	Cluster string `json:"cluster"`
	ServiceDescriptionURL string `json:"serviceDescriptionURL"`
	SleepTimeSeconds int64 `json:"sleepTimeSeconds"`
}
