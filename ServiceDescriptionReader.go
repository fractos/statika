package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"strings"
	"github.com/aws/aws-sdk-go/service/s3"
)

// New creates a new instance of the service description reader for this passed filename
func NewS3ServiceDescriptionReader(config string) *ServiceDescriptionReader {
	if strings.HasPrefix(config, "s3://") {
		return &S3ServiceDescriptionReader{config: config}
	}
	return nil
} // NewS3ServiceDescriptionReader

func (s3ServiceDescriptionReader *ServiceDescriptionReader) Read() []*ServiceDescription {
	var services []*ServiceDescription
	if err:= json.Unmarshal(data, &services); err != nil {
		return nil
	}

	return services
} // Read
