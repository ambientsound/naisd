package api

import (
	"encoding/json"
	"fmt"
	"github.com/Jeffail/gabs"
	"github.com/prometheus/client_golang/prometheus"
	"io/ioutil"
	"net/http"
	"strconv"
	"bytes"
	"strings"
	"github.com/golang/glog"
)

func init() {
	prometheus.MustRegister(httpReqsCounter)
}

type ResourcePayload struct {
	Alias 		string
	Properties	Properties
	Scope		Scope
	Type		string
}

type ApplicationInstancePayload struct {
	Application			string
	Environment			string
	Version				string
	ExposedResources	[]int
	UsedResources		[]int
}

type FasitClient struct {
	FasitUrl string
	Username string
	Password string
}

type Properties struct {
	Url			string
	EndpointUrl	string
	WsdlUrl		string
	Username	string
	Description	string
}

type Scope struct {
	Environment	string
	Zone		string
}

type Password struct {
	Ref string
}

type FasitResource struct {
	Alias        string
	ResourceType string                 `json:"type"`
	Properties   map[string]string
	Secrets      map[string]map[string]string
	Certificates map[string]interface{} `json:"files""`
}

type ResourceRequest struct {
	Alias        string
	ResourceType string
}

type NaisResource struct {
	id           int
	name         string
	resourceType string
	properties   map[string]string
	secret       map[string]string
	certificates map[string][]byte
}

func (fasit FasitClient) GetResources(resourcesRequests []ResourceRequest, environment string, application string, zone string) (resources []NaisResource, err error) {
	for _, request := range resourcesRequests {
		resource, appError := fasit.getResource(request, environment, application, zone)
		if appError != nil {
			return []NaisResource{}, fmt.Errorf("Failed to get resource: %s", appError.Message)
		}
		resources = append(resources, resource)
	}
	return resources, nil
}

func (fasit FasitClient) createApplicationInstance(deploymentRequest NaisDeploymentRequest, exposedResourceIds, usedResourceIds []int) error {
	requestCounter.With(nil).Inc()



	payload, err := json.Marshal(buildApplicationInstancePayload(deploymentRequest, exposedResourceIds, usedResourceIds))
	if err != nil {
		errorCounter.WithLabelValues("create_request").Inc()
		return fmt.Errorf("Unable to create payload (%s)", err)
	}

	req, err := http.NewRequest("POST", fasit.FasitUrl+"/api/v2/applicationinstances/", bytes.NewBuffer(payload))
	if err != nil {
		errorCounter.WithLabelValues("create_request").Inc()
		return fmt.Errorf("Unable to create request: %s", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)

	httpReqsCounter.WithLabelValues(strconv.Itoa(resp.StatusCode), "POST").Inc()
	if resp.StatusCode != 200 {
		errorCounter.WithLabelValues("error_fasit").Inc()
		return fmt.Errorf("Fasit returned: %s (%s)", body, strconv.Itoa(resp.StatusCode))
	}

	return nil
}

func (fasit FasitClient) createOrUpdateFasitResources(resources []ExposedResource, hostname, environment, application, zone string) ([]int, error) {
	var exposedResourceIds []int
	for _, resource := range resources {
		var request = ResourceRequest{Alias: resource.Alias, ResourceType: resource.ResourceType}
		existingResource, appError := fasit.getResource(request, environment, application, zone)

		if appError.Code == 404 {
			// Create new resource if none was found
			createdResourceId, err := fasit.createResource(resource, environment, application, zone)
			if err != nil {
				return nil, fmt.Errorf("Failed creating resource: %s of type %s with path %s. (%s)", resource.Alias, resource.ResourceType, resource.Path, err)
			}
			exposedResourceIds = append(exposedResourceIds, createdResourceId)
		} else if appError != nil {
			// Failed contacting Fasit
			return nil, fmt.Errorf("Encountered a problem while contacting Fasit (%s)", appError.Error)
		}
		// Updating Fasit resource
		updatedResourceId, err := fasit.updateResource(existingResource.id, resource, environment, application, zone)
		if err != nil {
			return nil, fmt.Errorf("Failed creating resource: %s of type %s with path %s. (%s)", resource.Alias, resource.ResourceType, resource.Path, err)
		}
		exposedResourceIds = append(exposedResourceIds, updatedResourceId)
	}
	return exposedResourceIds, nil
}

func getResourceIds(usedResources []NaisResource) (usedResourceIds []int) {
	for _, resource := range usedResources {
		usedResourceIds = append(usedResourceIds, resource.id)
	}
	return usedResourceIds
}

func environmentExistsInFasit(fasitUrl string, deploymentRequest NaisDeploymentRequest) error {
	fasit := FasitClient{fasitUrl, deploymentRequest.Username, deploymentRequest.Password}
	return fasit.getFasitEnvironment(deploymentRequest.Environment)
}

func applicationExistsInFasit(fasitUrl string, deploymentRequest NaisDeploymentRequest) (error) {
	fasit := FasitClient{fasitUrl, deploymentRequest.Username, deploymentRequest.Password}
	return fasit.getFasitApplication(deploymentRequest.Application)
}

func fetchFasitResources(fasitUrl string, deploymentRequest NaisDeploymentRequest, appConfig NaisAppConfig) ([]NaisResource, error) {
	var resourceRequests []ResourceRequest
	for _, resource := range appConfig.FasitResources.Used {
		resourceRequests = append(resourceRequests, ResourceRequest{Alias: resource.Alias, ResourceType: resource.ResourceType})
	}

	fasit := FasitClient{fasitUrl, deploymentRequest.Username, deploymentRequest.Password}

	return fasit.GetResources(resourceRequests, deploymentRequest.Environment, deploymentRequest.Application, deploymentRequest.Zone)
}

// Updates Fasit with information
func updateFasit(fasitUrl string, deploymentRequest NaisDeploymentRequest, resources []NaisResource, appConfig NaisAppConfig, hostname string) error {
	fasit := FasitClient{fasitUrl, deploymentRequest.Username, deploymentRequest.Password}

	usedResourceIds := getResourceIds(resources)

	exposedResourceIds, err := fasit.createOrUpdateFasitResources(appConfig.FasitResources.Exposed, hostname, deploymentRequest.Environment, deploymentRequest.Application, deploymentRequest.Zone)
	if 	err != nil {
		return err
	}

	glog.Infof("exposed: %s\nused: %s", exposedResourceIds, usedResourceIds)

	if err := fasit.createApplicationInstance(deploymentRequest, exposedResourceIds, usedResourceIds); err != nil {
		return err
	}

	return nil
}

func (fasit FasitClient) getResource(resourcesRequest ResourceRequest, environment string, application string, zone string) (NaisResource, *appError) {
	requestCounter.With(nil).Inc()
	req, err := buildResourceRequest(fasit.FasitUrl, resourcesRequest.Alias, resourcesRequest.ResourceType, environment, application, zone)
	if err != nil {
		errorCounter.WithLabelValues("create_request").Inc()
		return NaisResource{}, &appError{err, "Failed creating Fasit request", 500}
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	defer resp.Body.Close()

	if err != nil {
		errorCounter.WithLabelValues("contact_fasit").Inc()
		return NaisResource{}, &appError{err, "Error contacting Fasit", 500}
	}

	body, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		errorCounter.WithLabelValues("read_body").Inc()
		return NaisResource{}, &appError{err, "Could not read body", 500}
	}

	httpReqsCounter.WithLabelValues(strconv.Itoa(resp.StatusCode), "GET").Inc()
	if resp.StatusCode == 404 {
		errorCounter.WithLabelValues("error_fasit").Inc()
		return NaisResource{}, &appError{err, "Resource not found in Fasit", 404}
	}

	if resp.StatusCode > 299 {
		errorCounter.WithLabelValues("error_fasit").Inc()

		return NaisResource{}, &appError{err, "Unexpected error returned from Fasit",resp.StatusCode}
	}


	var fasitResource FasitResource

	err = json.Unmarshal(body, &fasitResource)
	if err != nil {
		errorCounter.WithLabelValues("unmarshal_body").Inc()
		return NaisResource{}, &appError{err, "Could not unmarshal Fasit response", 500}
	}

	resource, err := fasit.mapToNaisResource(fasitResource)

	if err != nil {
		return NaisResource{}, &appError{err, "Could not map response to Nais resource ", 500}
	}

	return resource, nil
}

func (fasit FasitClient) createResource(resource ExposedResource, environment, application, zone string) (int, error) {
	requestCounter.With(nil).Inc()

	payload, err := buildResourcePayload(resource, environment, zone)
	if err != nil {
		errorCounter.WithLabelValues("create_request").Inc()
		return 0, fmt.Errorf("Unable to create payload (%s)", err)
	}

	req, err := http.NewRequest("POST", fasit.FasitUrl+"/api/v2/resources/", bytes.NewBuffer(payload))
	if err != nil {
		errorCounter.WithLabelValues("create_request").Inc()
		return 0, fmt.Errorf("Unable to create request: %s", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)

	httpReqsCounter.WithLabelValues(strconv.Itoa(resp.StatusCode), "POST").Inc()
	if resp.StatusCode > 299 {
		errorCounter.WithLabelValues("error_fasit").Inc()
		return 0, fmt.Errorf("Fasit returned: %s (%s)", body, strconv.Itoa(resp.StatusCode))
	}

	type CreatedResource struct {
		Id int
	}
	createdResource := new(CreatedResource)
	if err := json.Unmarshal(body, &createdResource); err != nil {
		errorCounter.WithLabelValues("read_body").Inc()
		return 0, fmt.Errorf("Could not read response: %s", err)
	}

	return createdResource.Id, nil
}
func (fasit FasitClient) updateResource(existingResourceId int, resource ExposedResource, environment, application, zone string) (int, error) {
	requestCounter.With(nil).Inc()

	payload, err := buildResourcePayload(resource, environment, zone)
	if err != nil {
		errorCounter.WithLabelValues("create_request").Inc()
		return 0, fmt.Errorf("Unable to create payload (%s)", err)
	}

	req, err := http.NewRequest("PUT", fasit.FasitUrl+"/api/v2/resources/", bytes.NewBuffer(payload))
	if err != nil {
		errorCounter.WithLabelValues("create_request").Inc()
		return 0, fmt.Errorf("Unable to create request: %s", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)

	httpReqsCounter.WithLabelValues(strconv.Itoa(resp.StatusCode), "POST").Inc()
	if resp.StatusCode > 299 {
		errorCounter.WithLabelValues("error_fasit").Inc()
		return 0, fmt.Errorf("Fasit returned: %s (%s)", body, strconv.Itoa(resp.StatusCode))
	}

	type CreatedResource struct {
		Id int
	}
	createdResource := new(CreatedResource)
	if err := json.Unmarshal(body, &createdResource); err != nil {
		errorCounter.WithLabelValues("read_body").Inc()
		return 0, fmt.Errorf("Could not read response: %s", err)
	}

	return createdResource.Id, nil
}

func (fasit FasitClient) getFasitEnvironment(environmentName string) error {
	requestCounter.With(nil).Inc()
	req, err := http.NewRequest("GET", fasit.FasitUrl+"/api/v2/environments/"+environmentName, nil)
	if err != nil {
		return fmt.Errorf("Could not create request: %s", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)

	defer resp.Body.Close()

	if err != nil {
		return fmt.Errorf("Error contacting Fasit: %s", err)
	}

	if resp.StatusCode == 200 {
		return nil
	}
	return fmt.Errorf("Could not find environment %s in Fasit", environmentName)
}

func (fasit FasitClient) getFasitApplication(application string) (error) {
	requestCounter.With(nil).Inc()
	req, err := http.NewRequest("GET", fasit.FasitUrl+"/api/v2/applications/"+application, nil)
	if err != nil {
		return fmt.Errorf("Could not create request: %s", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)

	defer resp.Body.Close()

	if err != nil {
		return fmt.Errorf("Error contacting Fasit: %s", err)
	}

	if resp.StatusCode == 200 {
		return nil
	}
	return fmt.Errorf("Could not find application %s in Fasit", application)
}

func (fasit FasitClient) mapToNaisResource(fasitResource FasitResource) (resource NaisResource, err error) {
	resource.name = fasitResource.Alias
	resource.resourceType = fasitResource.ResourceType
	resource.properties = fasitResource.Properties

	if len(fasitResource.Secrets) > 0 {
		secret, err := resolveSecret(fasitResource.Secrets, fasit.Username, fasit.Password)
		if err != nil {
			errorCounter.WithLabelValues("resolve_secret").Inc()
			return NaisResource{}, fmt.Errorf("Unable to resolve secret: %s", err)
		}
		resource.secret = secret
	}

	if fasitResource.ResourceType == "certificate" && len(fasitResource.Certificates) > 0 {
		files, err := resolveCertificates(fasitResource.Certificates, fasitResource.Alias)

		if err != nil {
			errorCounter.WithLabelValues("resolve_file").Inc()
			return NaisResource{}, fmt.Errorf("unable to resolve Certificates: %s", err)
		}

		resource.certificates = files

	} else if fasitResource.ResourceType == "applicationproperties" {
		for _, prop := range strings.Split(fasitResource.Properties["applicationProperties"], "\r\n") {
			parts := strings.SplitN(prop, "=", 2)
			resource.properties[parts[0]] = parts[1]
		}
		delete(resource.properties, "applicationProperties")
	}

	return resource, nil
}
func resolveCertificates(files map[string]interface{}, resourceName string) (map[string][]byte, error) {
	fileContent := make(map[string][]byte)

	fileName, fileUrl, err := parseFilesObject(files)
	if err != nil {
		return fileContent, err
	}

	response, err := http.Get(fileUrl)
	if err != nil {
		errorCounter.WithLabelValues("contact_fasit").Inc()
		return fileContent, fmt.Errorf("error contacting fasit when resolving file: %s", err)
	}
	defer response.Body.Close()

	bodyBytes, err := ioutil.ReadAll(response.Body)
	if err != nil {
		errorCounter.WithLabelValues("contact_fasit").Inc()
		return fileContent, fmt.Errorf("error downloading file: %s", err)
	}

	fileContent[resourceName+"_"+fileName] = bodyBytes
	return fileContent, nil

}

func parseFilesObject(files map[string]interface{}) (fileName string, fileUrl string, e error) {
	json, err := gabs.Consume(files)
	if err != nil {
		errorCounter.WithLabelValues("error_fasit").Inc()
		return "", "", fmt.Errorf("Error parsing fasit json: %s ", files)
	}

	fileName, fileNameFound := json.Path("keystore.filename").Data().(string)
	if !fileNameFound {
		errorCounter.WithLabelValues("error_fasit").Inc()
		return "", "", fmt.Errorf("Error parsing fasit json. Filename not found: %s ", files)
	}

	fileUrl, fileUrlfound := json.Path("keystore.ref").Data().(string)
	if !fileUrlfound {
		errorCounter.WithLabelValues("error_fasit").Inc()
		return "", "", fmt.Errorf("Error parsing fasit json. Fileurl not found: %s ", files)
	}

	return fileName, fileUrl, nil
}

func resolveSecret(secrets map[string]map[string]string, username string, password string) (map[string]string, error) {

	req, err := http.NewRequest("GET", secrets[getFirstKey(secrets)]["ref"], nil)

	if err != nil {
		return map[string]string{}, err
	}

	req.SetBasicAuth(username, password)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		errorCounter.WithLabelValues("contact_fasit").Inc()
		return map[string]string{}, fmt.Errorf("Error contacting fasit when resolving secret: %s", err)
	}

	httpReqsCounter.WithLabelValues(strconv.Itoa(resp.StatusCode), "GET").Inc()
	if resp.StatusCode > 299 {
		errorCounter.WithLabelValues("error_fasit").Inc()
		return map[string]string{}, fmt.Errorf("Fasit gave errormessage when resolving secret: %s" + strconv.Itoa(resp.StatusCode))
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)

	return map[string]string{"password": string(body)}, nil
}

func getFirstKey(m map[string]map[string]string) string {
	if len(m) > 0 {
		for key := range m {
			return key
		}
	}
	return ""
}

func buildResourceRequest(fasit string, alias string, resourceType string, environment string, application string, zone string) (*http.Request, error) {
	req, err := http.NewRequest("GET", fasit+"/api/v2/scopedresource", nil)

	q := req.URL.Query()
	q.Add("alias", alias)
	q.Add("type", resourceType)
	q.Add("environment", environment)
	q.Add("application", application)
	q.Add("zone", zone)
	req.URL.RawQuery = q.Encode()
	return req, err
}

func generateScope(resource ExposedResource, environment, zone string) Scope {
	if resource.AllZones {
		return Scope{
			Environment:environment,
		}
	}
	return Scope{
		Environment:environment,
		Zone: zone,
	}
}
func buildApplicationInstancePayload(deploymentRequest NaisDeploymentRequest, exposedResourceIds, usedResourceIds []int) ApplicationInstancePayload {
		return ApplicationInstancePayload{
			Application: deploymentRequest.Application,
			Environment: deploymentRequest.Environment,
			Version: deploymentRequest.Version,
			ExposedResources: exposedResourceIds,
			UsedResources: usedResourceIds,
		}
}

func buildResourcePayload(resource ExposedResource, environment, zone string) ([]byte, error) {
	var resourcePayload ResourcePayload

	switch resource.ResourceType {
	case "RestService":
		resourcePayload = ResourcePayload{
			Type: resource.ResourceType,
			Alias: resource.Alias,
			Properties:Properties{
				Url:resource.Path,
				Description: resource.Description,
			},
			Scope: generateScope(resource, environment, zone),
		}
	case "WebserviceEndpoint":
		resourcePayload = ResourcePayload{
			Type: resource.ResourceType,
			Alias: resource.Alias,
			Properties:Properties{
				EndpointUrl: resource.Path,
				WsdlUrl:fmt.Sprintf("http://maven.adeo.no/nexus/service/local/artifact/maven/redirect?r=m2internal&g=%s&a=%s&v=%s&e=zip", resource.WsdlGroupId, resource.WsdlArtifactId, resource.WsdlVersion),
				Description: resource.Description,
			},
			Scope: generateScope(resource, environment, zone),
		}
	}
	// Reference of valid resources in Fasit
	// ['DataSource', 'MSSQLDataSource', 'DB2DataSource', 'LDAP', 'BaseUrl', 'Credential', 'Certificate', 'OpenAm', 'Cics', 'RoleMapping', 'QueueManager', 'WebserviceEndpoint', 'RestService', 'WebserviceGateway', 'EJB', 'Datapower', 'EmailAddress', 'SMTPServer', 'Queue', 'Topic', 'DeploymentManager', 'ApplicationProperties', 'MemoryParameters', 'LoadBalancer', 'LoadBalancerConfig', 'FileLibrary', 'Channel

	if payload, err := json.Marshal(resourcePayload); err != nil {
		return []byte{}, err
	} else {
		return payload, nil
	}
}

var httpReqsCounter = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Subsystem: "fasitAdapter",
		Name:      "http_requests_total",
		Help:      "How many HTTP requests processed, partitioned by status code and HTTP method.",
	},
	[]string{"code", "method"})

var requestCounter = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Subsystem: "fasit",
		Name:      "requests",
		Help:      "Incoming requests to fasitadapter",
	},
	[]string{})

var errorCounter = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Subsystem: "fasit",
		Name:      "errors",
		Help:      "Errors occurred in fasitadapter",
	},
	[]string{"type"})
