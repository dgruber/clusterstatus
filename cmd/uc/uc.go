/*
   Copyright 2014 Daniel Gruber, Univa, My blog http://www.gridengine.eu

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
	"fmt"
	"github.com/dgruber/ubercluster/pkg/output"
	"github.com/dgruber/ubercluster/pkg/staging"
	"gopkg.in/alecthomas/kingpin.v1"
	"io/ioutil"
	"log"
	"os"
)

// Disable logging by default
func init() {
	log.SetOutput(ioutil.Discard)
}

var (
	app       = kingpin.New("uc", "A tool which can interact with multiple compute clusters.")
	verbose   = app.Flag("verbose", "Enables enhanced logging for debugging.").Bool()
	cluster   = app.Flag("cluster", "Cluster name to interact with.").Default("default").String()
	otp       = app.Flag("otp", "One time password (\"yubikey\") or shared secret.").Default("").String()
	outformat = app.Flag("format", "Output format specifier (default/json).").Default("default").String()

	show               = app.Command("show", "Displays information about connected clusters.")
	showJob            = show.Command("job", "Information about a particular job.")
	showJobStateId     = showJob.Flag("state", "Show only jobs in that state (r/q/h/s/R/Rh/d/f/u/all).").Default("all").String()
	showJobId          = showJob.Arg("id", "Id of job").Default("").String()
	showJobUser        = showJob.Flag("user", "Shows only jobs of a particular user.").Default("").String()
	showMachine        = show.Command("machine", "Information about compute hosts.")
	showMachineName    = showMachine.Arg("name", "Name of machine (or \"all\" for all.").Default("all").String()
	showQueue          = show.Command("queue", "Information about queues.")
	showQueueName      = showQueue.Arg("name", "Name of queue to show.").Default("all").String()
	showCategories     = show.Command("category", "Information about job categories.")
	showCategoriesName = showCategories.Arg("name", "Name of job category to show.").Default("all").String()
	showSession        = show.Command("session", "Information about job sessions.")
	showSessionName    = showSession.Arg("name", "Name of the job session to show.").Default("all").String()

	run         = app.Command("run", "Submits an application to a cluster.")
	runCommand  = run.Arg("command", "Command to submit.").Default("#nocommand#").String()
	runArg      = run.Flag("arg", "Argument of the command (use \" when having spaces).").Default("").String()
	runName     = run.Flag("name", "Reference name of the command.").Default("").String()
	runQueue    = run.Flag("queue", "Queue name for the job.").Default("").String()
	runCategory = run.Flag("category", "Job category / job class of the job.").Default("").String()
	alg         = run.Flag("alg", "Automatic cluster selection when submitting jobs (\"rand\", \"prob\", \"load\")").Default("").String()
	fileUp      = run.Flag("upload", "Path to job which is uploaded before execution.").Default("").String()

	runlocal        = app.Command("runlocal", "Runs a command as child of the proxy.")
	runlocalCommand = runlocal.Arg("command", "Command to run.").Required().String()
	runlocalArg     = runlocal.Flag("arg", "Argument of the command (use \" when having spaces.)").Default("").String()

	// operations on job
	terminate      = app.Command("terminate", "Terminate operation.")
	terminateJob   = terminate.Command("job", "Terminates (ends) a job in a cluster.")
	terminateJobId = terminateJob.Arg("jobid", "Id of the job to terminate.").Default("").String()

	suspend      = app.Command("suspend", "Suspend operation.")
	suspendJob   = suspend.Command("job", "Suspends (pauses) a job in a cluster.")
	suspendJobId = suspendJob.Arg("jobid", "Id of the job to suspend.").Default("").String()

	resume      = app.Command("resume", "Resume operation.")
	resumeJob   = resume.Command("job", "Resumes a suspended job in a cluster.")
	resumeJobId = resumeJob.Arg("jobid", "Id of the job to resume.").Default("").String()

	// filestaging interface
	fs          = app.Command("fs", "Filesystem interface")
	fsLs        = fs.Command("ls", "List all files in staging area.")
	fsUp        = fs.Command("up", "Upload files to staging area.")
	fsUpFiles   = fsUp.Arg("files", "Path to files to upload.").Required().Strings()
	fsDown      = fs.Command("down", "Download files from staging area.")
	fsDownFiles = fsDown.Arg("files", "Filenames to download from staging area.").Required().Strings()

	// configuration
	cfg     = app.Command("config", "Configuration of cluster proxies.")
	cfgList = cfg.Command("list", "Lists all configured cluster proxies.")

	// uc as proxy itself
	incpt     = app.Command("inception", "Run uc as compatible proxy itself. Allows to create trees of clusters.")
	incptPort = incpt.Arg("port", "Address to bind uc http server to.").Default(":8989").String()
)

func main() {
	arguments := os.Args[1:]
	if len(arguments) == 0 {
		arguments = append(arguments, "--help")
	}

	p := kingpin.MustParse(app.Parse(arguments))

	if *verbose {
		log.SetOutput(os.Stdout)
	}

	// read in configuration
	ReadConfig()

	// based on cluster name or selection algorithm
	// create the address to send requests
	clusteraddress, clustername, err := selectClusterAddress(*cluster, *alg)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	// output can be produced in different formats
	of := output.MakeOutputFormater(*outformat)

	// read in one time password in case of yubikey
	var yubi bool
	if *otp == "yubikey" {
		yubi = true
		*otp = getYubiKey()
	} else {
		yubi = false
	}

	switch p {
	case showJob.FullCommand():
		if showJobId != nil && *showJobId != "" {
			log.Println("showJobId: ", *showJobId)
			showJobDetails(clusteraddress, *showJobId, of)
		} else {
			showJobs(clusteraddress, *showJobStateId, *showJobUser, of)
		}
	case cfgList.FullCommand():
		listConfig(clusteraddress)
	case showMachine.FullCommand():
		showMachines(clusteraddress, *showMachineName, of)
	case showQueue.FullCommand():
		showQueues(clusteraddress, *showQueueName, of)
	case showCategories.FullCommand():
		showJobCategories(clusteraddress, "ubercluster", *showCategoriesName)
	case showSession.FullCommand():
		showJobSessions(clusteraddress, *showSessionName)
	case run.FullCommand():
		if *fileUp != "" {
			staging.FsUploadFile(*otp, clusteraddress, "ubercluster", *fileUp)
			if yubi {
				*otp = getYubiKey() // we need another one time password for submission
			}
		}
		submitJob(clusteraddress, clustername, *runName, *runCommand, *runArg, *runQueue, *runCategory, *otp)
	case runlocal.FullCommand():
		runLocalRequest(*otp, clusteraddress, *runlocalCommand, *runlocalArg)
	case terminateJob.FullCommand():
		performOperation(clusteraddress, "ubercluster", "terminate", *terminateJobId)
	case suspendJob.FullCommand():
		performOperation(clusteraddress, "ubercluster", "suspend", *suspendJobId)
	case resumeJob.FullCommand():
		performOperation(clusteraddress, "ubercluster", "resume", *resumeJobId)
	case fsLs.FullCommand():
		staging.FsListFiles(*otp, clusteraddress, "ubercluster", of)
	case fsUp.FullCommand():
		staging.FsUploadFiles(*otp, clusteraddress, "ubercluster", *fsUpFiles, of)
	case fsDown.FullCommand():
		staging.FsDownloadFiles(*otp, clusteraddress, "ubercluster", *fsDownFiles, of)
	case incpt.FullCommand():
		inceptionMode(*incptPort)
	}
}
