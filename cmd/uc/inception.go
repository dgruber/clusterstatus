/*
   Copyright 2015 Daniel Gruber, Univa, My blog: http://www.gridengine.eu

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

// Run uc as proxy itself. Allows to stack clusters of cluster recursively.

import (
	"errors"
	"fmt"
	"github.com/dgruber/ubercluster/pkg/persistency"
	"github.com/dgruber/ubercluster/pkg/proxy"
	"github.com/dgruber/ubercluster/pkg/types"
	"log"
	"strings"
	"sync"
)

type Inception struct {
	inceptionAddress string // address of uc itself
	config           Config // uc configuration object
	request          *Request
}

func NewInception(certFile, keyFile string, otp string, config Config) *Inception {
	return &Inception{
		config:  config, // configuration contains all connected clusters,
		request: NewRequest(certFile, keyFile, &otp),
	}
}

// Implements the ProxyImplementer interface

// collects jobinfos from all clusters in parallel
type jiProtected struct {
	sync.Mutex
	sync.WaitGroup
	jobinfos []types.JobInfo
}

// requestJobInfos requests job infos of jobs in the
// given state from a cluster given by the address
func requestJobInfos(i *Inception, ji *jiProtected, state string, address string) {
	log.Println("Requesting from: ", address)
	jis := i.request.GetJobs(address, state, "")
	log.Println("Got following jobinfos: ", jis)
	if jis != nil {
		ji.Lock()
		ji.jobinfos = append(ji.jobinfos, jis...)
		ji.Unlock()
	}
	ji.Done()
}

func (i *Inception) GetJobInfosByFilter(filtered bool, filter types.JobInfo) []types.JobInfo {
	var jip jiProtected
	jip.jobinfos = make([]types.JobInfo, 0, 0)
	jip.Add(len(i.config.Cluster))
	// request clusters in parallel and wait for all of them
	for _, c := range i.config.Cluster {
		if addr := fmt.Sprintf("%s/", c.Address); addr == i.inceptionAddress {
			log.Println("Skipping own address ", c.Address)
			jip.Done()
			continue
		}
		go requestJobInfos(i, &jip, "all", fmt.Sprintf("%s/v1", c.Address))
	}
	// wait until we got all job infos from all cluster
	jip.Wait()

	return jip.jobinfos
}

func getJobFromCluster(i *Inception, clustername string, jobid string) (*types.JobInfo, error) {
	// check if cluster name is known
	address := ""
	version := "v1"
	for _, c := range i.config.Cluster {
		if c.Name == clustername {
			address = c.Address
			version = c.ProtocolVersion
			break
		}
	}
	if address != "" {
		request := fmt.Sprintf("%s%s", address, version)
		log.Println("GetJobFromCluster request", request)
		job, err := i.request.GetJob(request, jobid)
		if err == nil {
			return &job, nil
		}
		log.Println("error during requesting job: ", err)
		return nil, err

	}
	return nil, errors.New("Couldn't find clustername in config: " + clustername)
}

func (i *Inception) GetJobInfo(jobid string) *types.JobInfo {
	// search job id in all connected clusters
	// if it has a postfix - only in that cluster
	// 1301@mybiggridenginecluster search 1301 in the given cluster
	if strings.Contains(jobid, "@") {
		// get cluster name
		jobAtCluster := strings.Split(jobid, "@")
		if len(jobAtCluster) == 2 {
			job, _ := getJobFromCluster(i, jobAtCluster[1], jobAtCluster[0])
			return job
		}
		log.Println("Wrong job identifier (expected jobid@cluster or jobid) but is ", jobid)
	} else {
		// request default cluster for the given job identifier
		job, _ := getJobFromCluster(i, "default", jobid)
		return job
	}
	return nil
}

func (i *Inception) GetAllMachines(machines []string) ([]types.Machine, error) {
	allmachines := make([]types.Machine, 0, 0)
	for _, c := range i.config.Cluster {
		log.Println("Requesting from: ", c.Address)
		// we don't request our own address...
		if addr := fmt.Sprintf("%s/", c.Address); addr == i.inceptionAddress {
			continue
		}
		address, _, err := GetClusterAddress(c.Name)
		if err != nil {
			log.Panicln(err.Error())
			return nil, err
		}
		if ms, err := i.request.GetMachines(address, "all"); err == nil {
			allmachines = append(allmachines, ms...)
			log.Println("Appending: ", allmachines)
		} else {
			log.Println("Error while requesting machines from ", c.Name, err)
		}
		// TODO filter according request
		// TODO remove duplicates
	}
	return allmachines, nil
}

// GetAllQueues returns all queue names from all clusters which are
// connected to the uc tool.
func (i *Inception) GetAllQueues(queues []string) ([]types.Queue, error) {
	allqueues := make([]types.Queue, 0, 0)
	// TODO go functions of course
	for _, c := range i.config.Cluster {
		log.Println("Requesting from: ", c.Address)
		// we don't request our own address...
		if addr := fmt.Sprintf("%s/", c.Address); addr == i.inceptionAddress {
			continue
		}
		address, _, err := GetClusterAddress(c.Name)
		if err != nil {
			log.Panicln(err.Error())
			return nil, err
		}
		if qs, err := i.request.GetQueues(address, "all"); err == nil {
			allqueues = append(allqueues, qs...)
			log.Println("Appending: ", allqueues)
		} else {
			log.Println("Error while requesting queues from ", c.Name, err)
		}
		// TODO filter according request
		// TODO remove duplicates
	}
	return allqueues, nil
}

func (i *Inception) GetAllSessions(session []string) ([]string, error) {
	// TODO implement
	allsessions := make([]string, 0, 0)
	log.Println("GetAllSessions() not implemented")
	return allsessions, nil
}

func (i *Inception) GetAllCategories() ([]string, error) {
	cat := make([]string, 0, 0)
	for _, c := range i.config.Cluster {
		log.Println("Requesting from: ", c.Address)
		if addr := fmt.Sprintf("%s/", c.Address); addr == i.inceptionAddress {
			log.Println("Skipping own address")
			continue
		}
		address, _, err := GetClusterAddress(c.Name)
		if err != nil {
			log.Panicln(err.Error())
			return nil, err
		}
		cat = append(cat, i.request.GetJobCategories(address, "ubercluster", "all")...)
	}
	return cat, nil
}

func (i *Inception) DRMSVersion() string {
	return "0.1"
}

func (i *Inception) DRMSName() string {
	return "ubercluster"
}

func (i *Inception) DRMSLoad() float64 {
	return 0.5
}

func (i *Inception) RunJob(template types.JobTemplate) (string, error) {
	return "", nil
}

func (i *Inception) JobOperation(jobsessionname, operation, jobid string) (string, error) {
	return "", nil
}

// start uc as proxy
func inceptionMode(certFile, keyFile, otp, address string) {
	incept := NewInception(certFile, keyFile, otp, config)

	fmt.Println("Starting uc in inception mode as proxy listening at address: ", address)
	var sc proxy.SecConfig
	sc.OTP = otp
	var pi persistency.DummyPersistency
	// yubikey not supported since it would require interactivity
	proxy.ProxyListenAndServe(address, "", "", sc, &pi, incept)
}
