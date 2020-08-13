// Copyright 2020 Blake Covarrubias
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"log"
	"os"
	"path/filepath"

	homedir "github.com/mitchellh/go-homedir"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func initAuthCreds() *rest.Config {

	// Use Kubernetes service account for authentication when running in-cluster
	var config *rest.Config
	if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token"); !os.IsNotExist(err) {
		config, err = rest.InClusterConfig()
		if err != nil {
			panic(err.Error())
		}
		// Use kubeconfig for authentication when running out-of-cluster
	} else {
		// Uses the current context in kubeconfig
		config, err = clientcmd.BuildConfigFromFlags(master, kubeconfig)
		if err != nil {
			log.Fatalln("Failed to read kubeconfig:", err)
		}
	}

	return config
}

func kubeconfigPath() string {
	var kubeconfigPath string
	if home, _ := homedir.Dir(); home != "" {
		kubeconfigPath = filepath.Join(home, ".kube", "config")
	} else {
		kubeconfigPath = ""
	}

	return kubeconfigPath
}

func newK8sClient() (*kubernetes.Clientset, error) {
	// Creates the k8sClient
	config := initAuthCreds()
	k8sClient, err := kubernetes.NewForConfig(config)

	if err != nil {
		return nil, err
	}
	return k8sClient, nil
}
