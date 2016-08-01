/*
Copyright 2016 Skippbox, Ltd All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package loader

import (
	"github.com/Sirupsen/logrus"
	"github.com/docker/libcompose/docker"
	"github.com/docker/libcompose/project"
	"github.com/skippbox/kompose/pkg/kobject"
	"strconv"
	"strings"
	"github.com/docker/libcompose/lookup"
	"os"
	"github.com/docker/libcompose/config"
	"path/filepath"
)

// load Environment Variable from compose file
func loadEnvVarsfromCompose(e map[string]string) []kobject.EnvVar {
	envs := []kobject.EnvVar{}
	for k, v := range e {
		envs = append(envs, kobject.EnvVar{
			Name:  k,
			Value: v,
		})
	}
	return envs
}

// Load Ports from compose file
func loadPortsFromCompose(composePorts []string) ([]kobject.Ports, string) {
	ports := []kobject.Ports{}
	character := ":"
	for _, port := range composePorts {
		p := kobject.ProtocolTCP
		if strings.Contains(port, character) {
			hostPort := port[0:strings.Index(port, character)]
			hostPort = strings.TrimSpace(hostPort)
			hostPortInt, err := strconv.Atoi(hostPort)
			if err != nil {
				return nil, "Invalid host port of " + port
			}
			containerPort := port[strings.Index(port, character)+1:]
			containerPort = strings.TrimSpace(containerPort)
			containerPortInt, err := strconv.Atoi(containerPort)
			if err != nil {
				return nil, "Invalid container port of " + port
			}
			ports = append(ports, kobject.Ports{
				HostPort:      int32(hostPortInt),
				ContainerPort: int32(containerPortInt),
				Protocol:      p,
			})
		} else {
			containerPortInt, err := strconv.Atoi(port)
			if err != nil {
				return nil, "Invalid container port of " + port
			}
			ports = append(ports, kobject.Ports{
				ContainerPort: int32(containerPortInt),
				Protocol:      p,
			})
		}

	}
	return ports, ""
}

// load Docker Compose file into KomposeObject
func LoadCompose(file string) kobject.KomposeObject {
	komposeObject := kobject.KomposeObject{
		ServiceConfigs: make(map[string]kobject.ServiceConfig),
	}
	context := &docker.Context{}
	if file == "" {
		file = "docker-compose.yml"
	}
	context.ComposeFiles = []string{file}

	if context.ResourceLookup == nil {
			context.ResourceLookup = &lookup.FileResourceLookup{}
		}

		if context.EnvironmentLookup == nil {
			cwd, err := os.Getwd()
			if err != nil {
				return kobject.KomposeObject{}
			}
			context.EnvironmentLookup = &lookup.ComposableEnvLookup{
				Lookups: []config.EnvironmentLookup{
					&lookup.EnvfileLookup{
						Path: filepath.Join(cwd, ".env"),
					},
					&lookup.OsEnvLookup{},
				},
			}
		}

	// load compose file into composeObject
	composeObject := project.NewProject(&context.Context, nil, nil)
	err := composeObject.Parse()
	if err != nil {
		logrus.Fatalf("Failed to load compose file", err)
	}

	// transform composeObject into komposeObject
	composeServiceNames := composeObject.ServiceConfigs.Keys()

	// volume config and network config are not supported
	if len(composeObject.NetworkConfigs) > 0 {
		logrus.Warningf("Unsupported network configuration of compose v2 - ignoring")
	}
	if len(composeObject.VolumeConfigs) > 0 {
		logrus.Warningf("Unsupported volume configuration of compose v2 - ignoring")
	}

	networksWarningFound := false

	for _, name := range composeServiceNames {
		if composeServiceConfig, ok := composeObject.ServiceConfigs.Get(name); ok {
			//FIXME: networks always contains one default element, even it isn't declared in compose v2.
			if len(composeServiceConfig.Networks.Networks) > 0 &&
				composeServiceConfig.Networks.Networks[0].Name != "default" &&
				!networksWarningFound {
				logrus.Warningf("Unsupported key networks - ignoring")
				networksWarningFound = true
			}
			kobject.CheckUnsupportedKey(composeServiceConfig)
			serviceConfig := kobject.ServiceConfig{}
			serviceConfig.Image = composeServiceConfig.Image
			serviceConfig.ContainerName = composeServiceConfig.ContainerName

			// load environment variables
			envs := loadEnvVarsfromCompose(composeServiceConfig.Environment.ToMap())
			serviceConfig.Environment = envs

			// load ports
			ports, err := loadPortsFromCompose(composeServiceConfig.Ports)
			if err != "" {
				logrus.Fatalf("Failed to load ports from compose file: " + err)
			}
			serviceConfig.Port = ports

			serviceConfig.WorkingDir = composeServiceConfig.WorkingDir
			serviceConfig.Volumes = composeServiceConfig.Volumes

			// convert compose labels to annotations
			serviceConfig.Annotations = map[string]string(composeServiceConfig.Labels)

			serviceConfig.CPUSet = composeServiceConfig.CPUSet
			serviceConfig.CPUShares = composeServiceConfig.CPUShares
			serviceConfig.CPUQuota = composeServiceConfig.CPUQuota
			serviceConfig.CapAdd = composeServiceConfig.CapAdd
			serviceConfig.CapDrop = composeServiceConfig.CapDrop
			serviceConfig.Expose = composeServiceConfig.Expose
			serviceConfig.Privileged = composeServiceConfig.Privileged
			serviceConfig.Restart = composeServiceConfig.Restart
			serviceConfig.User = composeServiceConfig.User

			komposeObject.ServiceConfigs[name] = serviceConfig
		}
	}

	return komposeObject
}
