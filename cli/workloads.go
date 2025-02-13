package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"slices"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert/yaml"
	"github.com/synadia-labs/nex/client"
	"github.com/synadia-labs/nex/models"
)

type Workload struct {
	Start StartWorkload `cmd:"" name:"start" help:"Run a workload on a target node" aliases:"run,deploy"`
	Stop  StopWorkload  `cmd:"" name:"stop" help:"Stop a running workload" aliases:"undeploy"`
	List  ListWorkload  `cmd:"" name:"list" help:"List workloads" aliases:"ls"`
	// Info  InfoWorkload  `cmd:"" name:"info" help:"Get information about a workload"`
	Copy CloneWorkload `cmd:"" name:"clone" help:"Copy a workload to another node" aliases:"cp,copy"`
	// Bundle BundleWorkload `cmd:"" help:"Bundles a workload into an OCI artifact" aliases:"build,package"`
}

type (
	StartWorkload struct {
		// Options for directly tasking workload to node; requires system namespace
		NodeId      string `help:"Node ID to run the workload on"`
		NodePubXKey string `name:"node-xkey-pub" help:"Node public xkey used for encryption"`
		// Options for auction starting a workload
		AuctionTags map[string]string `name:"tags" help:"Node tags to run the workload on; --node-id will take precedence"`

		AgentType           string `name:"type" help:"Type of workload" default:"native"`
		WorkloadName        string `name:"name" help:"Name of the workload"`
		WorkloadDescription string `name:"description" help:"Description of the workload"`
		WorkloadLifecycle   string `name:"lifecycle" help:"Runtype of the workload: service, function, job" default:"service" enum:"service,function,job"`

		// This will need to validate against start request provided by agent at registration
		WorkloadStartRequest json.RawMessage `name:"start-request" placeholder:"{}" help:"Start request for the workload"`
		WorkloadNexfile      *os.File        `name:"nexfile" short:"f" placeholder:"Nexfile" help:"Nexfile for the workload; formatted in YAML"`
	}
	StopWorkload struct {
		WorkloadId string `arg:"" name:"id" help:"ID of the workload to stop"`
	}
	ListWorkload struct {
		AgentType    string   `name:"type" help:"Type of workload" placeholder:"native"`
		ShowMetadata bool     `name:"show-metadata" default:"false" help:"Show metadata for workloads"`
		Filter       []string `name:"filter" help:"Workload filter sent to agent for processing" placeholder:"state"`
	}
	// InfoWorkload struct{}
	CloneWorkload struct {
		WorkloadId  string            `arg:"" name:"id" help:"ID of the workload to stop"`
		AuctionTags map[string]string `name:"tags" help:"Node tags to run the workload on"`
		StopOrig    bool              `name:"stop" default:"false" help:"Stop the original workload after cloning"`
	}
)

func (r *StartWorkload) Run(globals *Globals) error {
	if r.NodeId != "" && globals.Namespace != models.NodeSystemNamespace {
		return errors.New("node-id can only be used with system namespace")
	}

	nc, err := configureNatsConnection(globals)
	if err != nil {
		return err
	}

	if nc == nil {
		return errors.New("no NATS connection available")
	}

	client := client.NewClient(nc, globals.Namespace)

	deploymentId := r.NodeId

	if r.WorkloadNexfile == nil {
		if info, err := os.Stat("./Nexfile"); err == nil && !info.IsDir() {
			f, err := os.Open("./Nexfile")
			if err != nil {
				return err
			}
			r.WorkloadNexfile = f
		}
	}

	var startRequest interface{}
	if r.WorkloadNexfile != nil {
		defer r.WorkloadNexfile.Close()

		data, err := io.ReadAll(r.WorkloadNexfile)
		if err != nil {
			return err
		}

		err = yaml.Unmarshal(data, &startRequest)
		if err != nil {
			return err
		}
	}

	if r.NodeId == "" {
		aucResp, err := client.Auction(r.AgentType, r.AuctionTags)
		if err != nil {
			return err
		}

		randomNode := aucResp[rand.Intn(len(aucResp))]

		if !slices.Contains(randomNode.SupportedLifecycles, models.WorkloadLifecycle(r.WorkloadLifecycle)) {
			return errors.New("agent does not support requested lifecycle")
		}

		compiler := jsonschema.NewCompiler()
		sch, err := jsonschema.UnmarshalJSON(bytes.NewReader([]byte(randomNode.StartRequestSchema)))
		if err != nil {
			return err
		}
		err = compiler.AddResource("schema.json", sch)
		if err != nil {
			return err
		}

		schema, err := compiler.Compile("schema.json")
		if err != nil {
			return err
		}

		if startRequest != nil {
			err = schema.Validate(startRequest)
			if err != nil {
				return err
			}
		} else if r.WorkloadStartRequest == nil && r.WorkloadNexfile == nil {
			// TODO: create an interactive mode to fill out start request
			// if schema.Properties == nil {
			// 	return errors.New("schema has no properties")
			// }
			// for fieldName, fieldSchema := range schema.Properties {
			// 	fmt.Printf("%s: %s\n", fieldName, fieldSchema.Types.String())
			// }

			return errors.New("interactive start request not yet implemented")
		}
		deploymentId = randomNode.BidderId
	}

	r.WorkloadStartRequest, err = json.Marshal(startRequest)
	if err != nil {
		return err
	}

	startResponse, err := client.StartWorkload(deploymentId, r.WorkloadName, r.WorkloadDescription, string(r.WorkloadStartRequest), r.AgentType, models.WorkloadLifecycle(r.WorkloadLifecycle))
	if err != nil {
		return err
	}

	fmt.Printf("Workload %s [%s] successfully started\n", startResponse.Name, startResponse.Id)
	return nil
}

func (r *StartWorkload) Validate() error {
	var errs error
	return errs
}

func (s *StopWorkload) Run(globals *Globals) error {
	nc, err := configureNatsConnection(globals)
	if err != nil {
		return err
	}

	if nc == nil {
		return errors.New("no NATS connection available")
	}

	stopResponse, err := client.NewClient(nc, globals.Namespace).StopWorkload(s.WorkloadId)
	if err != nil {
		return err
	}

	if globals.JSON {
		stopResponseB, err := json.Marshal(stopResponse)
		if err != nil {
			return err
		}
		fmt.Println(string(stopResponseB))
		return nil
	}

	fmt.Printf("Workload %s successfully stopped\n", stopResponse.Id)
	return nil
}

func (r *ListWorkload) Run(globals *Globals) error {
	nc, err := configureNatsConnection(globals)
	if err != nil {
		return err
	}

	if nc == nil {
		return errors.New("no NATS connection available")
	}

	resp, err := client.NewClient(nc, globals.Namespace).ListWorkloads(r.Filter)
	if err != nil {
		return err
	}

	if globals.JSON {
		respB, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		fmt.Println(string(respB))
		return nil
	}

	workloads := 0
	if len(resp) > 0 {
		tW := table.NewWriter()
		tW.SetStyle(table.StyleRounded)
		tW.Style().Title.Align = text.AlignCenter
		tW.Style().Format.Header = text.FormatDefault
		tW.SetTitle("Running Workloads - " + globals.Namespace)
		if r.ShowMetadata {
			tW.AppendHeader(table.Row{"Id", "Name", "Start Time", "Execution Time", "Type", "Lifecycle", "State", "Metadata"})
		} else {
			tW.AppendHeader(table.Row{"Id", "Name", "Start Time", "Execution Time", "Type", "Lifecycle", "State"})
		}
		for _, agentResponse := range resp {
			for _, workload := range *agentResponse {
				rt := workload.Runtime
				if workload.WorkloadLifecycle != "function" {
					rt = "--"
				}

				if r.ShowMetadata {
					meta := "--"
					if workload.Metadata != nil {
						metaB, err := json.Marshal(workload.Metadata)
						if err == nil {
							meta = string(metaB)
						}
					}
					tW.AppendRow(table.Row{workload.Id, workload.Name, workload.StartTime, rt, workload.WorkloadType, workload.WorkloadLifecycle, workload.WorkloadState, meta})
				} else {
					tW.AppendRow(table.Row{workload.Id, workload.Name, workload.StartTime, rt, workload.WorkloadType, workload.WorkloadLifecycle, workload.WorkloadState})
				}
				workloads++
			}
		}

		if workloads > 0 {
			fmt.Println(tW.Render())
			return nil
		}
	}

	fmt.Println("No workloads found")
	return nil
}

func (r *CloneWorkload) Run(globals *Globals) error {
	nc, err := configureNatsConnection(globals)
	if err != nil {
		return err
	}

	if nc == nil {
		return errors.New("no NATS connection available")
	}

	nexClient := client.NewClient(nc, globals.Namespace)
	resp, err := nexClient.CloneWorkload(r.WorkloadId, r.AuctionTags)
	if err != nil {
		return err
	}

	var stopped bool
	if r.StopOrig {
		stopResp, err := nexClient.StopWorkload(r.WorkloadId)
		if err != nil {
			return err
		} else if stopResp.Stopped {
			stopped = true
		}
	}

	if globals.JSON {
		respB, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		fmt.Println(string(respB))
		return nil
	}

	fmt.Printf("Workload %s [%s] successfully started\n", resp.Name, resp.Id)
	if stopped {
		fmt.Printf("Original workload [%s] stopped\n", r.WorkloadId)
	}

	return nil
}
