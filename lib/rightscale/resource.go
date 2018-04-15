package rightscale

import (
	"encoding/json"
	"github.com/pkg/errors"
	"strings"
	"strconv"
	"os"
)

type serverInstance struct {
	Actions []struct {
		Rel string `json:"rel"`
	} `json:"actions"`
	AssociatePublicIPAddress bool `json:"associate_public_ip_address"`
	CloudSpecificAttributes struct {
		AutomaticInstanceStoreMapping bool   `json:"automatic_instance_store_mapping"`
		EbsOptimized                  bool   `json:"ebs_optimized"`
		IamInstanceProfile            string `json:"iam_instance_profile"`
		PlacementTenancy              string `json:"placement_tenancy"`
	} `json:"cloud_specific_attributes"`
	CreatedAt           string        `json:"created_at"`
	IPForwardingEnabled bool          `json:"ip_forwarding_enabled"`
	Links               rsLinks       `json:"links"`
	Locked              bool          `json:"locked"`
	Name                string        `json:"name"`
	PricingType         string        `json:"pricing_type"`
	PrivateIPAddresses  []interface{} `json:"private_ip_addresses"`
	PublicIPAddresses   []interface{} `json:"public_ip_addresses"`
	ResourceUID         string        `json:"resource_uid"`
	State               string        `json:"state"`
	UpdatedAt           string        `json:"updated_at"`
}

type rawTagList struct {
	Links rsLinks `json:"links"`
	Tags []struct {
		Name string `json:"name"`
	} `json:"tags"`
}

type rawTagListSlice []rawTagList

type rsLink struct {
	Href string `json:"href"`
	Rel  string `json:"rel"`
}

type rsLinks []rsLink

type ServerArray struct {
	Actions []struct {
		Rel string `json:"rel"`
	} `json:"actions"`
	ArrayType   string `json:"array_type"`
	Description string `json:"description"`
	Href        string
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

type ServerArrays []ServerArray

type tag struct {
	Name  string
	Value string
}

type tags []tag

func (c Client) Arrays(withTags ...bool) (arrayList ServerArrays, e error) {
	arrayListRequestParams := RequestParams{
		method: "GET",
		url:    "/api/server_arrays?view=instance_detail",
	}
	var data []byte
	var err error
	if mockRSCalls() {
		data = arrayListResponseMock
	} else {
		data,err = c.Request(arrayListRequestParams)
		if err != nil {
			return ServerArrays{}, errors.Errorf("encountered error requesting server arrays %s",err)
		}
	}
	json.Unmarshal(data, &arrayList)

	if len(withTags) > 0 && withTags[0] {
		arrayList, err = c.PopulateArrayTags(arrayList)
		if err != nil {
			return ServerArrays{}, errors.Errorf("encountered error attempting %s", err)
		}
	}
	return
}

func (c Client) PopulateArrayTags(arrayList ServerArrays) (ServerArrays, error) {
	selfHrefFunc := func(a ServerArray) string {
		return a.Links.linkValue("self")
	}
	refs := arrayList.collect(selfHrefFunc)
	tags, err := c.getTags(refs)
	if err != nil {
		return ServerArrays{}, errors.Errorf("encountered error requesting tags for server arrays %s", err)
	}
	arrayTags := tags.mapToArrayHREF()
	return arrayList.associateArrayTags(arrayTags), nil
}

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

	if mockRSCalls() {
		data = tagListResponseMock
	} else {
		data, err = c.Request(tagRequestParams)
		if err != nil {
			return rawTagListSlice{}, errors.Errorf("encountered error requesting server array tags %s", err)
		}
	}
	err = json.Unmarshal(data, &tagList)
	if err != nil {
		return rawTagListSlice{}, errors.Errorf("encountered error unmarshalling tag response %s", err)
	}
	return tagList, nil
}

func (t tags) TagValue(name string) string {
	for _, x := range t {
		if x.Name == name {
			return x.Value
		}
	}
	return "N/A"
}

//transforms Rightscales obnoxious tag response to a toplevel object
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

func extractRSEC2Tag(t string) (key, value string, e error) {
	tagParts := strings.Split(t, ":")
	if tagParts[0] != "ec2" {
		e = errors.New("was not EC2 tag")
		return
	}
	p2 := strings.Split(tagParts[1], "=")
	return p2[0], p2[1], nil
}

func (sa ServerArrays) associateArrayTags(tagMap map[string]tags) ServerArrays {
	var sa2 ServerArrays
	for _, array := range sa {
		href := array.Links.linkValue("self")
		t, ok := tagMap[href]
		if ok {
			array.ArrayTags = t
		}
		array.Href = href
		sa2 = append(sa2, array)
	}
	return sa2
}

func (links rsLinks) linkValue(name string) string {
	for _, link := range links {
		if link.Rel == name {
			return link.Href
		}
	}
	return ""
}

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
