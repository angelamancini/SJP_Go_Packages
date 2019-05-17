package rightscale

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
)

// ServerArray represents a single array in Rightscale
type ServerArray struct {
	Actions []struct {
		Rel string `json:"rel"`
	} `json:"actions"`
	ArrayType        string `json:"array_type"`
	Description      string `json:"description"`
	Href             string
	ElasticityParams struct {
		AlertSpecificParams struct {
			DecisionThreshold  string `json:"decision_threshold"`
			VotersTagPredicate string `json:"voters_tag_predicate"`
		} `json:"alert_specific_params"`
		Bounds struct {
			MaxCount string `json:"max_count"`
			MinCount string `json:"min_count"`
		} `json:"bounds"`
		Pacing struct {
			ResizeCalmTime string `json:"resize_calm_time"`
			ResizeDownBy   string `json:"resize_down_by"`
			ResizeUpBy     string `json:"resize_up_by"`
		} `json:"pacing"`
		ScheduleEntries []struct {
			Day      string `json:"day"`
			MaxCount int    `json:"max_count"`
			MinCount int    `json:"min_count"`
			Time     string `json:"time"`
		} `json:"schedule_entries"`
	} `json:"elasticity_params"`
	InstancesCount int            `json:"instances_count"`
	Links          rsLinks        `json:"links"`
	Name           string         `json:"name"`
	NextInstance   serverInstance `json:"next_instance"`
	State          string         `json:"state"`
	ArrayTags      tags
}

// ServerArrays represent a collection of ServerArray resources
type ServerArrays []ServerArray

// serverInstance represents a single server instance
type serverInstance struct {
	Actions []struct {
		Rel string `json:"rel"`
	} `json:"actions"`
	AssociatePublicIPAddress bool `json:"associate_public_ip_address"`
	CloudSpecificAttributes  struct {
		AutomaticInstanceStoreMapping bool   `json:"automatic_instance_store_mapping"`
		EbsOptimized                  bool   `json:"ebs_optimized"`
		IamInstanceProfile            string `json:"iam_instance_profile"`
		PlacementTenancy              string `json:"placement_tenancy"`
	} `json:"cloud_specific_attributes"`
	CreatedAt           string   `json:"created_at"`
	IPForwardingEnabled bool     `json:"ip_forwarding_enabled"`
	Links               rsLinks  `json:"links"`
	Locked              bool     `json:"locked"`
	Name                string   `json:"name"`
	PricingType         string   `json:"pricing_type"`
	PrivateIPAddresses  []string `json:"private_ip_addresses"`
	PublicIPAddresses   []string `json:"public_ip_addresses"`
	ResourceUID         string   `json:"resource_uid"`
	State               string   `json:"state"`
	UpdatedAt           string   `json:"updated_at"`
}

// ServerInstances represents a collection of servicerInstance reosurces
type ServerInstances []serverInstance

// rawTagList Represents collection of resource tags in their raw form prior to being parsed as name/value pairs
type rawTagList struct {
	Links rsLinks `json:"links"`
	Tags  []struct {
		Name string `json:"name"`
	} `json:"tags"`
}

// rawTagListSlice Represents slice of collection of resource tags in their raw form prior to being parsed as name/value pairs
type rawTagListSlice []rawTagList

// rsLink Represents Rightscale link which point to related resources
type rsLink struct {
	Href string `json:"href"`
	Rel  string `json:"rel"`
}

// rsLinks Represents a collection of Rightscale links which are pointers to related resources
type rsLinks []rsLink

// tag Represents a single tag, which is just a name/value pair
type tag struct {
	Name  string
	Value string
}

// tags Represents a collection of tags which are just name/value pairs
type tags []tag

// Deployment Represents a single deployment in Rightscale
type Deployment struct {
	Name  string  `json:"name"`
	Links rsLinks `json:"links"`
}

// Deployments Represents a collection of deployment resources in Rightscale
type Deployments []Deployment

// Input Represents a single name/value pair
type Input struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

// Inputs represents a slice of Input which represents a single name/value pair
type Inputs []Input

func timeTrack(start time.Time, name string) {
	elapsed := time.Since(start)
	log.Printf("%s took %s", name, elapsed)
}

// Arrays returns a list of arrays for a given Rightscale Account
// The Arrays function accepts a single optional boolean parameter which instructs
// it to also retrieve tag data for the given result set. getting tags for the full
// set of arrays is a somewhat expensive operation
// this function makes use of Goroutines and channels to speed up the retrieval of arrays,
// this speedup is achieved by breaking the array request into multiple request group by rightscale deployment
func (c Client) Arrays(withTags ...bool) (arrayList ServerArrays, e error) {
	var wantTags bool
	if len(withTags) > 0 && withTags[0] {
		wantTags = true
	} else {
		wantTags = false
	}
	var serverArrayHrefs []string
	deploymentList, err := c.GetDeployments()
	if err != nil {
		return arrayList, err
	}
	for _, deployment := range deploymentList {
		withDetail := fmt.Sprintf("%s?%s", deployment.Links.LinkValue("server_arrays"), "view=instance_detail")
		serverArrayHrefs = append(serverArrayHrefs, withDetail)
	}
	ch := make(chan ServerArray)
	var results ServerArrays
	var loopGroup sync.WaitGroup
	go func(arrays chan ServerArray) {
		for a := range arrays {
			results = append(results, a)
		}
	}(ch)
	for _, arrayHref := range serverArrayHrefs {
		loopGroup.Add(1)
		go func(href string, getTags bool, x *sync.WaitGroup, zzz chan ServerArray) {
			sa, err := c.getArrays(href, getTags)
			if err != nil {
				log.Printf("Could not get arrays from %s - Error: %s", href, err)
			}
			//loop through arrays and push then into channel
			for _, array := range sa {
				//push here
				zzz <- array
			}

			x.Done()
		}(arrayHref, wantTags, &loopGroup, ch)

	}
	loopGroup.Wait()
	close(ch)
	return results, nil
}

// getArrays returns all server arrays from a specified URL.
// Deployments in Rightscale contain an Href to the arrays contained within them,
// this url is used to pull the arrays. This function accepts an optional withTags
// boolean parameter which indicates that it should pull in array meta data also
func (c Client) getArrays(url string, withTags ...bool) (arrayList ServerArrays, e error) {
	defer timeTrack(time.Now(), url)
	// arrayListRequestParams := RequestParams{
	// 	method: "GET",
	// 	url:    url,
	// }
	//could not find symbol value for msg
	var data []byte
	var err error
	// if mockRSCalls() {
	// 	data = arrayListResponseMock
	// } else {
	// 	data, err = c.Request(arrayListRequestParams)
	// 	if err != nil {
	// 		return ServerArrays{}, errors.Errorf("encountered error requesting"+
	// 			" server arrays to retrieve tags %s", err)
	// 	}
	// }
	err = json.Unmarshal(data, &arrayList)
	if err != nil {
		return nil, errors.Errorf("could not unmarshal json from get array api call %s", err)
	}
	//todo, ensure href being set like for single call
	if len(withTags) > 0 && withTags[0] {
		//If deployment contained no arrays we cannot further process things
		if len(arrayList) == 0 {
			return ServerArrays{}, nil
		}
		arrayList, err = c.PopulateArrayTags(arrayList)
		if err != nil {
			return ServerArrays{}, errors.Errorf("encountered error attempting %s", err)
		}
	}
	return
}

// ArraysParallel is now an alias for Arrays, they use to be two different functions,
// the implementation from parallel became the default
func (c Client) ArraysParallel(withTags ...bool) (arrayList ServerArrays, e error) {
	return c.Arrays(withTags...)
}

// GetDeployments returns a all Deployments in the Rightscale account
func (c Client) GetDeployments() (Deployments, error) {
	//get list of deployments in account
	deploymentListParams := RequestParams{
		method: "GET",
		url:    "/api/deployments",
	}
	data, err := c.Request(deploymentListParams)
	var deploymentList Deployments
	if err != nil {
		return deploymentList, errors.New("encountered error getting deployment list")
	}
	err = json.Unmarshal(data, &deploymentList)
	if err != nil {
		return nil, errors.Errorf("could not unmarshal json from get deployment api call %s", err)
	}
	return deploymentList, nil
}

// Array retrieves a single Array by its numeric ID.
// If you have an Array's href split by / and take the last part.
// that is the ID. todo, validate format of string provided to ensure href is not passed in
// Errors returned by this function will be from failed network calls or parsing the returned Json
func (c Client) Array(arrayID string, withTags ...bool) (array ServerArray, e error) {
	arrayRequestParams := RequestParams{
		method: "GET",
		url:    fmt.Sprintf("/api/server_arrays/%s?view=instance_detail", arrayID),
	}

	data, err := c.Request(arrayRequestParams)
	if err != nil {
		return ServerArray{}, errors.Errorf("encountered error requesting server arrays %s", err)
	}
	err = json.Unmarshal(data, &array)
	if err != nil {
		return ServerArray{}, errors.WithMessage(err, "could not unmarshal Array response")
	}
	array.Href = array.id()

	if len(withTags) > 0 && withTags[0] {
		arrayList := ServerArrays{array}
		arrayList, err = c.PopulateArrayTags(arrayList)
		if err != nil {
			return ServerArray{}, errors.Errorf("encountered error attempting to get tags %s", err)
		}
		array = arrayList[0]
	}
	return
}

// LaunchArrayInstances makes calls to rightscale to launch count number of instances
// The Count parameter should probably always be reasonable <20?
// Errors returned by this function will be from failed network calls to Rightscale or unexpected response status code
// 200 and 201 are the only expected status codes for this call
func (c Client) LaunchArrayInstances(array ServerArray, count int) error {
	path := fmt.Sprintf("%s/%s?count=%d&api_behavior=sync", array.Href, "launch", count)
	arrayLaunchParams := RequestParams{"POST", path, nil}
	resp, err := c.RequestDetailed(arrayLaunchParams)
	if err != nil {
		return errors.WithMessage(err, "Error calling launch array endpoint")
	}
	if resp.StatusCode == 201 || resp.StatusCode == 200 {
		return nil
	}
	responseBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.Errorf("Error calling launch array endpoint expected 201 got %d", resp.StatusCode)
	}
	return errors.Errorf("Error calling launch array endpoint expected 201 got %d"+
		" - %s", resp.StatusCode, string(responseBody))
}

// DownscaleArrayInstances makes calls to Rightscale to terminate count number of instances in an Array
// This function calls the TerminateInstance function to actually do the dirty work, this is just a wrapper
// to control the amount and ensure the right set of instances are being killed.
// This function factors instance age into the decision making process with a bias to killing older instances first
// Errors returned by this function are the ones bubbled up from the TerminateInstances
func (c Client) DownscaleArrayInstances(array ServerArray, count int) error {
	var createdAtDateMap = make(map[int]serverInstance) //this will stop working one day as unix timestamp will overflow 32 bits
	var createdAtDateSlice []int
	aid, _ := array.ArrayID()
	instances, err := c.GetArrayInstances(aid)
	if len(instances) < count {
		return errors.Errorf("Count submitted for downscale %d higher than running count %d", count, len(instances))
	}
	if err != nil {
		return errors.Errorf("Could not list array instances. Error %s", err)
	}
	//sort by created at time and select count oldest to send for termination
	// date format 2012/12/24 13:27:58 +0000
	for _, i := range instances {
		if i.State == "terminated" { //if an instance is in the terminated state then it should not be included
			continue
		}
		t, err := time.Parse("2006/01/02 15:04:05 -0700", i.CreatedAt)
		if err != nil {
			log.Println("Could not parse time format for Instance", i.Name)
		}
		createdAtDateMap[int(t.Unix())] = i
		createdAtDateSlice = append(createdAtDateSlice, int(t.Unix()))
	}
	sort.Ints(createdAtDateSlice)
	if count > len(createdAtDateSlice) {
		return errors.Errorf("Count submitted for downscale %d higher"+
			" than terminatable count in array %d", count, len(createdAtDateSlice))
	}
	terminatable := createdAtDateSlice[0:count]
	var terminatableHrefs []string
	for _, ts := range terminatable {
		instance, ok := createdAtDateMap[ts]
		if ok {
			log.Println("Instance being submitted for termination", instance.Name)
			terminatableHrefs = append(terminatableHrefs, instance.Links.LinkValue("self"))
		}
	}
	return c.TerminateInstances(terminatableHrefs)
}

// TerminateInstances terminates instances contained within the instanceHref slice
// Instances are terminated one by one in a loop and errors are all collected before returning
// Errors returned by this function will be from network failures, unexpected response status codes,
// or invalid IDs being passed in
func (c Client) TerminateInstances(instanceHrefs []string) error {
	var loopErrors []string
	for _, href := range instanceHrefs {
		log.Println("Terminating instance", href)
		path := fmt.Sprintf("%s/%s", href, "terminate")
		instanceTerminateParams := RequestParams{"POST", path, nil}
		resp, err := c.RequestDetailed(instanceTerminateParams)
		if err != nil {
			loopErrors = append(loopErrors, fmt.Sprintf("Error calling terminate instance endpoint for %s - %s", href, err))
		}
		if resp.StatusCode != 204 {
			responseBody, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				loopErrors = append(loopErrors, fmt.Sprintf("Error calling terminate instance endpoint for %s expected 204 got %d", href, resp.StatusCode))
			} else {
				loopErrors = append(loopErrors, fmt.Sprintf("Error calling terminate array endpoint for %s expected 204 got %d"+
					" - %s", href, resp.StatusCode, string(responseBody)))
			}
		}

	}
	if len(loopErrors) != 0 {
		return errors.Errorf("%s", strings.Join(loopErrors, " "))
	}
	return nil
}

// ArrayInputs retrieves a list of Inputs from a given array for the "next instance"
// This input set does not represent the inputs for currently running array instances
// if inputs from currently running array instances are needed, use the InstanceInputs function
func (c Client) ArrayInputs(array ServerArray) (inputList Inputs, e error) {
	nextInstance := array.Links.LinkValue("next_instance")
	inputListRequestParams := RequestParams{
		method: "GET",
		url:    fmt.Sprintf("%s/inputs", nextInstance),
	}
	data, err := c.Request(inputListRequestParams)
	if err != nil {
		return Inputs{}, errors.Errorf("encountered error requesting server array inputs %s", err)
	}
	err = json.Unmarshal(data, &inputList)
	if err != nil {
		return nil, errors.Errorf("could not unmarshal json from  array inputs api call %s", err)
	}
	inputs := Inputs{}
	for _, s := range inputs {
		valParts := strings.Split(s.Value, ":")
		inputType := valParts[0]
		inputValue := strings.Join(valParts[1:], ":")

		detail := Input{Name: s.Name, Type: inputType, Value: inputValue}
		inputs = append(inputs, detail)
	}
	return
}

// ArrayInputUpdate updates one input for the given array
// Inputs are updated for the "next instance" of an array
func (c Client) ArrayInputUpdate(array ServerArray, input Input) (e error) {
	newInput := map[string]string{}
	newInput[input.Name] = input.Value
	var body = map[string]map[string]string{}
	body["inputs"] = newInput
	nextInstance := array.Links.LinkValue("next_instance")
	updateInputsRequestParams := RequestParams{
		method: "PUT",
		url:    fmt.Sprintf("%s/inputs/multi_update", nextInstance),
		body:   body,
	}
	_, err := c.Request(updateInputsRequestParams)
	if err != nil {
		return errors.Errorf("encountered an error updating server araray inputs %s", err)
	}
	return
}

// InstanceInputs returns a current list of inputs from a single instance
// This function is not yet implemented, 😐
func (c Client) InstanceInputs(instance serverInstance) (Inputs, error) {
	return nil, errors.New("NOT YET IMPLEMENTED")
}

// GetArrayInstances returns a list of ServerInstances in a given array
// The arrayID parameter represents the last numeric portion of the href
// If you have an Array's href split by / and take the last part. that is the ID.
func (c Client) GetArrayInstances(arrayID string) (ServerInstances, error) {
	//todo, validate id format
	instanceListParams := RequestParams{
		method: "GET",
		url:    fmt.Sprintf("/api/server_arrays/%s/current_instances", arrayID),
	}
	var instances ServerInstances
	data, err := c.Request(instanceListParams)
	if err != nil {
		return instances, errors.Errorf("encountered error requesting server instances for array %s, %s", arrayID, err)
	}
	err = json.Unmarshal(data, &instances)
	if err != nil {
		return nil, errors.WithMessage(err, "error parsing response in get array instance function")
	}
	return instances, nil
}

// PopulateArrayTags take a list of Arrays and supplements their data with their tag information
// The return list represents the full list of arrays passed in
func (c Client) PopulateArrayTags(arrayList ServerArrays) (ServerArrays, error) {
	selfHrefFunc := func(a ServerArray) string {
		return a.Links.LinkValue("self")
	}
	refs := arrayList.collect(selfHrefFunc)
	//cannot further process
	if len(refs) == 0 {
		return arrayList, nil
	}
	tags, err := c.getTags(refs)
	if err != nil {
		return ServerArrays{}, errors.Errorf("encountered error requesting tags for server arrays %s", err)
	}
	arrayTags := tags.mapToArrayHREF()
	return arrayList.associateArrayTags(arrayTags), nil
}

// TagValue returns the tag value for a give tag name
// this is a helper function to traversing the tag struct
func (t tags) TagValue(name string) string {
	for _, x := range t {
		if x.Name == name {
			return x.Value
		}
	}
	return "N/A"
}

// getTags returns tags for a given set of HREFs representing a resource.
func (c Client) getTags(refs []string) (rawTagListSlice, error) {
	var tagList rawTagListSlice
	var body = make(map[string][]string)
	body["resource_hrefs"] = refs
	tagRequestParams := RequestParams{
		method: "POST",
		url:    "/api/tags/by_resource",
		body:   body,
	}
	_ = tagRequestParams

	var data []byte
	var err error

	// if mockRSCalls() {
	// 	data = tagListResponseMock
	// } else {
	// 	data, err = c.Request(tagRequestParams)
	// 	if err != nil {
	// 		return rawTagListSlice{}, errors.Errorf("encountered error requesting server array tags %s", err)
	// 	}
	// }
	err = json.Unmarshal(data, &tagList)
	if err != nil {
		return rawTagListSlice{}, errors.Errorf("encountered error attempting to unmarshal tag response %s", err)
	}
	return tagList, nil
}

// mapToArrayHREF transforms Rightscale's obnoxious tag response to a toplevel object
func (rawTagList rawTagListSlice) mapToArrayHREF() map[string]tags {
	var tagMap = make(map[string]tags) //toplevel tag list to return
	for _, rawTagItem := range rawTagList {
		tagList := rawTagItem.Tags //all
		var tagSet tags
		for _, tagItem := range tagList {
			name, value, err := extractRSEC2Tag(tagItem.Name)
			if err != nil {
				continue
			}
			tagSet = append(tagSet, tag{Name: name, Value: value})
		}
		for _, resourceLinks := range rawTagItem.Links {
			if resourceLinks.Rel == "resource" {
				tagMap[resourceLinks.Href] = tagSet
			}
		}
	}
	return tagMap
}

// extractRSEC2Tag identifies and returns ec2 tags from Rightscale resources
// EC2 tags on Rightscale resources have the prefix ec2
func extractRSEC2Tag(t string) (key, value string, e error) {
	tagParts := strings.Split(t, ":")
	if tagParts[0] != "ec2" {
		e = errors.New("was not EC2 tag")
		return
	}
	p2 := strings.Split(tagParts[1], "=")
	return p2[0], p2[1], nil
}

// associateArrayTags allows ServerArrays to be joined to their tag values
func (sa ServerArrays) associateArrayTags(tagMap map[string]tags) ServerArrays {
	var sa2 ServerArrays
	for _, array := range sa {
		href := array.id()
		t, ok := tagMap[href]
		if ok {
			array.ArrayTags = t
		}
		array.Href = href
		sa2 = append(sa2, array)
	}
	return sa2
}

// ArrayID returns the numeric portion at the end of an array's Href
// There is no reason for this function to return an error 🤦
func (sa ServerArray) ArrayID() (string, error) {
	stringParts := strings.Split(sa.id(), "/")
	idString := stringParts[len(stringParts)-1]
	return idString, nil
}

// id returns the value of the href named self -
// this represents the full Href as a path and not just the numeric portion
// if only the numeric portion of the href is desired use the ArrayID function
func (sa ServerArray) id() string {
	return sa.Links.LinkValue("self")
}

// LinkValues returns the value of a given link name,
// no error is returned from this function. if the name is not found an empty string "" is returned
func (links rsLinks) LinkValue(name string) string {
	for _, link := range links {
		if link.Rel == name {
			return link.Href
		}
	}
	return ""
}

// collect allows for the retrieval of a specific value from a whole set of server arrays
// if for example I wanted the hrefs for all server arrays I'd pass in a function that returns that string
// the populateArrayTags function makes use of this function refer to it for usage example
func (sa ServerArrays) collect(f func(ServerArray) string) (collectedSet []string) {
	//allows the collection of attributes from server
	for _, arr := range sa {
		returnValue := f(arr)
		if returnValue != "" {
			collectedSet = append(collectedSet, returnValue)
		}
	}
	return
}

func mockRSCalls() bool {
	x, _ := strconv.ParseBool(os.Getenv("MOCK_RS_CALLS"))
	return x
}
