package main

import (
	"encoding/json"
	"io/ioutil"
)

// New creates a new instance of the configuration reader for this passed filename
func NewConfigurationReader(filename string) *ConfigurationReader {
	return &ConfigurationReader{configFilename: filename}
} // NewConfigurationReader

func (configurationReader *ConfigurationReader) Read() (*Configuration, error) {
	data, err := ioutil.ReadFile(configurationReader.configFilename)
	if err != nil {
		return nil, err
	}

	var config *Configuration
	if err = json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return config, nil
} // Read

