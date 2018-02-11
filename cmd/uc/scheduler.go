/*
   Copyright 2015 Daniel Gruber, info@gridengine.eu

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

package main

import (
	"encoding/json"
	"fmt"
	"github.com/dgruber/ubercluster/pkg/http_helper"
	"log"
	"math"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

// just seed random number generator one time
var seeded bool = false

// Scheduler is an interface all scheduler needs to
// implement.
type Scheduler interface {
	SelectCluster() string
}

type SchedulerType int

const (
	ProbabilisticSchedulerType SchedulerType = iota
	RandomSchedulerType
	LoadBasedSchedulerType
)

type SchedulerImpl struct {
	Impl Scheduler
}

// MakeNewScheduler create a new scheduler implementation based
// on the SchedulerType and the cluster Config.
func MakeNewScheduler(st SchedulerType, config Config, client *http.Client) *SchedulerImpl {
	if seeded == false {
		rand.Seed(time.Now().UTC().UnixNano())
		seeded = true
	}
	var s SchedulerImpl
	switch st {
	case ProbabilisticSchedulerType:
		s.Impl = &ProbSched{
			conf:   config,
			client: client,
		}
	case RandomSchedulerType:
		s.Impl = &RandomSched{
			conf:   config,
			client: client,
		}
	case LoadBasedSchedulerType:
		s.Impl = &LoadBasedSched{
			conf:   config,
			client: client,
		}
	}
	return &s
}

// Implements the cluster selection algorithms.

type ProbSched struct {
	conf   Config
	client *http.Client
}

// probabilisticScheduler returns the name of the selected
// cluster from the configuration. The selection is based
// on the cluster load and selects a valid cluster (one
// with a lower load than 1). A cluster with a load of 0
// has a higher probability to be chosen than one with 0.9.
// If all clusters have the same load all of them have the
// same probability to be chosen.
func (ps *ProbSched) SelectCluster() string {
	// get load of each cluster
	selection := probabilisticSelection(getAllLoadValues(ps.conf, ps.client))
	if selection >= 0 {
		log.Printf("Selected cluster %s due to probabilistic selection.\n",
			ps.conf.Cluster[selection].Name)
		return ps.conf.Cluster[selection].Name
	}
	log.Println("No cluster selected, using default cluster.")
	return "default"
}

func probabilisticSelection(loads []float64) int {
	// invert the load to get a value which refledts the likelyhood
	// multiply by a large value (since we are choosing int random
	// numbers later on)
	// add the value to what we calculated for the cluster before
	if len(loads) <= 0 {
		return -1
	}
	likelyhood := make([]int64, len(loads), len(loads))
	for k, v := range loads {
		if k >= 1 {
			likelyhood[k] = likelyhood[k-1] + int64(((1.0 - v) * 10000))
		} else {
			likelyhood[k] = int64((1.0 - v) * 10000)
		}
	}
	// if all cluster reports 1.0 -> chose default cluster 0
	if likelyhood[len(loads)-1] <= 0 {
		return -1
	}
	// choose cluster depending on its likelyhood
	selection := rand.Int63n(likelyhood[len(loads)-1] - 1)
	for k, v := range likelyhood {
		if v > selection {
			return k
		}
	}
	return -1
}

type loadValues struct {
	sync.Mutex
	sync.WaitGroup
	load []float64
}

func getClusterLoad(lv *loadValues, index int, request string, client *http.Client) {
	if resp, err := http_helper.UberGet(client, *otp, request); err == nil {
		defer resp.Body.Close()
		decoder := json.NewDecoder(resp.Body)
		var load float64
		if err := decoder.Decode(&load); err != nil {
			lv.load[index] = load
		} else {
			log.Println("Error during decoding cluster load from ", request, err)
		}
	}
	lv.Done()
}

func getAllLoadValues(conf Config, client *http.Client) []float64 {
	var lv loadValues
	lv.load = make([]float64, len(conf.Cluster), len(conf.Cluster))
	lv.Add(len(conf.Cluster))
	for i := range conf.Cluster {
		addr := conf.Cluster[i].Address
		ver := conf.Cluster[i].ProtocolVersion
		go getClusterLoad(&lv, i, fmt.Sprintf("%s/%s/drmsload", addr, ver), client)
	}
	lv.Wait()
	return lv.load
}

func minLoad(load []float64) int {
	min := math.MaxFloat64
	index := 0
	for k, v := range load {
		if v < min {
			min = v
			index = k
		}
	}
	return index
}

type LoadBasedSched struct {
	conf   Config
	client *http.Client
}

// SelectCluster of the LoadBasedSched is a simple scheduler
// that selects the cluster with the lowest load.
func (lbs *LoadBasedSched) SelectCluster() string {
	// get all load values (time consuming)
	load := getAllLoadValues(lbs.conf, lbs.client)
	return lbs.conf.Cluster[minLoad(load)].Name
}

type RandomSched struct {
	conf   Config
	client *http.Client
}

// SelectCluster of the random scheduler selects a
// a cluster randomly and returns its name.
func (rs *RandomSched) SelectCluster() string {
	return rs.conf.Cluster[rand.Intn(len(rs.conf.Cluster))].Name
}
