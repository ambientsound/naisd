package api

import (
	"bytes"
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
	"strings"
	"testing"
	"io/ioutil"
	"fmt"
)

func TestGettingResource(t *testing.T) {

	alias := "alias1"
	resourceType := "datasource"
	environment := "environment"
	application := "application"
	zone := "zone"

	fasit := FasitClient{"https://fasit.local", "", ""}

	defer gock.Off()
	gock.New("https://fasit.local").
		Get("/api/v2/scopedresource").
		MatchParam("alias", alias).
		MatchParam("type", resourceType).
		MatchParam("environment", environment).
		MatchParam("application", application).
		MatchParam("zone", zone).
		Reply(200).File("testdata/fasitResponse.json")

	resource, err := fasit.getScopedResource(ResourceRequest{alias, resourceType}, environment, application, zone)

	assert.Nil(t, err)
	assert.Equal(t, alias, resource.name)
	assert.Equal(t, resourceType, resource.resourceType)
	assert.Equal(t, "jdbc:oracle:thin:@//a01dbfl030.adeo.no:1521/basta", resource.properties["url"])
	assert.Equal(t, "basta", resource.properties["username"])
}

func TestCreatingApplicationInstance(t *testing.T) {

	defer gock.Off()

	gock.New("https://fasit.local").
		Post("/api/v2/applicationinstances").
		Reply(201).
		BodyString("aiit")

	fasit := FasitClient{"https://fasit.local", "", ""}
	exposedResourceIds, usedResourceIds := []int{1, 2, 3}, []int{4, 5, 6}
	deploymentRequest := NaisDeploymentRequest{Application: "app", Environment: "env", Version: "123"}

	t.Run("A valid payload creates ApplicationInstance", func(t *testing.T) {
		err := fasit.createApplicationInstance(deploymentRequest, exposedResourceIds, usedResourceIds)
		assert.NoError(t, err)
	})

}
func TestCreatingResource(t *testing.T) {

	alias := "alias1"
	resourceType := "RestService"
	environment := "environment"
	application := "application"
	hostname := "hostname"
	zone := "zone"
	id := 4242
	exposedResource := ExposedResource{
		Alias:        alias,
		ResourceType: resourceType,
		Path:         "",
	}

	fasit := FasitClient{"https://fasit.local", "", ""}

	defer gock.Off()

	t.Run("createResource returns error if fasit is unreachable", func(t *testing.T) {
		_, err := fasit.createResource(exposedResource, environment, application, zone, hostname)
		assert.Error(t, err)
	})
	gock.New("https://fasit.local").
		Post("/api/v2/resources").
		Reply(201).
		JSON(map[string]int{"id": id})

	t.Run("createResource returns ID when called", func(t *testing.T) {
		createdResourceId, err := fasit.createResource(exposedResource, environment, application, zone, hostname)
		assert.NoError(t, err)
		assert.True(t, gock.IsDone())
		assert.Equal(t, id, createdResourceId)
	})
	gock.New("https://fasit.local").
		Post("/api/v2/resources").
		Reply(501).
		BodyString("bish")
	t.Run("createResource errs when Fasit fails", func(t *testing.T) {
		createdResourceId, err := fasit.createResource(exposedResource, environment, application, zone, hostname)
		assert.Error(t, err)
		assert.Equal(t, 0, createdResourceId)
	})
	gock.New("https://fasit.local").
		Post("/api/v2/resources").
		Reply(201).
		BodyString("bish")
	t.Run("createResource errs when Fasit returns gibberish", func(t *testing.T) {
		createdResourceId, err := fasit.createResource(exposedResource, environment, application, zone, hostname)
		assert.Error(t, err)
		assert.Equal(t, 0, createdResourceId)
	})
}

func TestUpdateResource(t *testing.T) {
	alias := "alias1"
	resourceType := "RestService"
	environment := "environment"
	application := "application"
	hostname := "hostname"
	zone := "zone"
	id := 4242
	exposedResource := ExposedResource{
		Alias:        alias,
		ResourceType: resourceType,
		Path:         "",
	}

	fasit := FasitClient{"https://fasit.local", "", "",}

	defer gock.Off()

	t.Run("updateResource returns error if fasit is unreachable", func(t *testing.T) {
		_, err := fasit.updateResource(id, exposedResource, environment, application, zone, hostname)
		assert.Error(t, err)
	})
	gock.New("https://fasit.local").
		Put("/api/v2/resources/" + fmt.Sprint(id)).
		Reply(200).
		JSON(map[string]int{"id": id})

	t.Run("updateResource returns ID when called", func(t *testing.T) {
		createdResourceId, err := fasit.updateResource(id, exposedResource, environment, application, zone, hostname)
		assert.NoError(t, err)
		assert.True(t, gock.IsDone())
		assert.Equal(t, id, createdResourceId)
	})
	gock.New("https://fasit.local").
		Put("/api/v2/resources/" + fmt.Sprint(id)).
		Reply(501).
		BodyString("bish")
	t.Run("updateResource errs when Fasit fails", func(t *testing.T) {
		createdResourceId, err := fasit.updateResource(id, exposedResource, environment, application, zone, hostname)
		assert.Error(t, err)
		assert.Equal(t, 0, createdResourceId)
	})
	gock.New("https://fasit.local").
		Put("/api/v2/resources/" + fmt.Sprint(id)).
		Reply(200).
		BodyString("bish")
	t.Run("updateResource errs when Fasit returns gibberish", func(t *testing.T) {
		createdResourceId, err := fasit.updateResource(id, exposedResource, environment, application, zone, hostname)
		assert.Error(t, err)
		assert.Equal(t, 0, createdResourceId)
	})
}

func TestGetLoadBalancerConfig(t *testing.T) {

	environment := "environment"
	application := "application"

	fasit := FasitClient{"https://fasit.local", "", ""}

	t.Run("Get load balancer config happy path", func(t *testing.T) {

		defer gock.Off()
		gock.New("https://fasit.local").
			Get("/api/v2/resources").
			MatchParam("environment", environment).
			MatchParam("application", application).
			MatchParam("type", "LoadBalancerConfig").
			Reply(200).File("testdata/fasitLbConfigResponse.json")

		resource, err := fasit.getLoadBalancerConfig("application", "environment")

		assert.NoError(t, err)
		assert.Equal(t, 2, len(resource.ingresses))
		assert.Equal(t, "LoadBalancerConfig", resource.resourceType)
	})

}
func TestGetResourceId(t *testing.T) {
	naisResources := []NaisResource{{id: 1}, {id: 2},}
	resourceIds := getResourceIds(naisResources)
	assert.Equal(t, []int{1, 2}, resourceIds)
}

type FakeFasitClient struct {
	FasitUrl string
	FasitClient
}

func (fasit FakeFasitClient) getScopedResource(resourcesRequest ResourceRequest, environment, application, zone string) (NaisResource, AppError) {
	switch application {
	case "notfound":
		return NaisResource{}, appError{fmt.Errorf("not found"), "Resource not found in Fasit", 404}
	case "fasitError":
		return NaisResource{}, appError{fmt.Errorf("error from fasit"), "random error", 500}
	default:
		return NaisResource{id: 1}, nil
	}
}

func (fasit FakeFasitClient) createResource(resource ExposedResource, environment, application, zone, hostname string) (int, error) {
	switch zone {
	case "failed":
		return 0, fmt.Errorf("random error")
	default:
		return 4242, nil
	}
}

var updateCalled bool

func (fasit FakeFasitClient) updateResource(existingResourceId int, resource ExposedResource, environment, application, zone, hostname string) (int, error) {
	updateCalled = true
	switch zone {
	case "failed":
		return 0, fmt.Errorf("random error")
	default:
		return 1, nil

	}
}
var createApplicationInstanceCalled bool

func (fasit FakeFasitClient) createApplicationInstance(deploymentRequest NaisDeploymentRequest, exposedResourceIds, usedResourceIds []int) error {
	createApplicationInstanceCalled = true
	return nil
}

func TestCreateOrUpdateFasitResources(t *testing.T) {

	alias := "alias1"
	resourceType := "RestService"
	environment := "environment"
	application := "application"
	zone := "zone"
	hostname := "bish"

	exposedResource := ExposedResource{
		Alias:        alias,
		ResourceType: resourceType,
		Path:         "",
	}
	exposedResources := []ExposedResource{exposedResource, exposedResource}

	fakeFasitClient := FakeFasitClient{}

	// Using application field to identify which response to return from getScopedResource on FakeFasitClient
	t.Run("Resources are created when their resource ID isn't found in Fasit", func(t *testing.T) {
		resourceIds, err := CreateOrUpdateFasitResources(fakeFasitClient, exposedResources, hostname, environment, "notfound", zone)
		assert.NoError(t, err)
		assert.Equal(t, []int{4242, 4242}, resourceIds)
	})
	t.Run("Returns an error if contacting Fasit fails", func(t *testing.T) {
		resourceIds, err := CreateOrUpdateFasitResources(fakeFasitClient, exposedResources, hostname, environment, "fasitError", zone)
		assert.Error(t, err)
		assert.Nil(t, resourceIds)
		assert.True(t, strings.Contains(err.Error(), "random error: error from fasit, (500)"))
	})

	// Using Zone field to identify which response to return from createResource on FakeFasitClient
	t.Run("Returns an error if unable to create resource", func(t *testing.T) {
		resourceIds, err := CreateOrUpdateFasitResources(fakeFasitClient, exposedResources, hostname, environment, "notfound", "failed")
		assert.Error(t, err)
		assert.Nil(t, resourceIds)
		assert.True(t, strings.Contains(err.Error(), "Failed creating resource: alias1 of type RestService with path . (random error)"))
	})
	t.Run("Updates Fasit if resources were found", func(t *testing.T) {
		updateCalled = false
		resourceIds, err := CreateOrUpdateFasitResources(fakeFasitClient, exposedResources, hostname, environment, application, zone)
		assert.NoError(t, err)
		assert.Equal(t, []int{1, 1}, resourceIds)
		assert.True(t, updateCalled)
	})
	// Using Zone field to identify which response to return from updateResource on FakeFasitClient
	t.Run("Returns an error if unable to update resource", func(t *testing.T) {
		resourceIds, err := CreateOrUpdateFasitResources(fakeFasitClient, exposedResources, hostname, environment, application, "failed")
		assert.Error(t, err)
		assert.Nil(t, resourceIds)
		assert.True(t, strings.Contains(err.Error(), "Failed updating resource: alias1 of type RestService with path . (random error)"))
	})
}

func TestResourceError(t *testing.T) {
	fasitClient := FasitClient{FasitUrl: "https://fasit.local"}
	defer gock.Off()
	gock.New("https://fasit.local").
		Get("/api/v2/scopedresource").
		Reply(404).BodyString("not found")

	resource, err := fetchFasitResources(fasitClient, NaisDeploymentRequest{Application: "app", Environment: "env", Version: "123"}, NaisAppConfig{FasitResources: FasitResources{Used: []UsedResource{{Alias: "resourcealias", ResourceType: "baseurl"}}}})
	assert.Error(t, err)
	assert.Empty(t, resource)
	assert.True(t, strings.Contains(err.Error(), "Resource not found in Fasit: (404)"))
}

func TestUpdateFasit(t *testing.T) {

	alias := "alias"
	resourceType := "restService"
	environment := "environment"
	application := "application"
	hostname := "bish"

	exposedResource := ExposedResource{
		Alias:        alias,
		ResourceType: resourceType,
		Path:         "",
	}

	usedResources := []NaisResource{{id: 1}, {id: 2}}
	exposedResources := []ExposedResource{exposedResource, exposedResource}

	deploymentRequest := NaisDeploymentRequest{
		Application: application,
		Environment: environment,
		Version:     version,
	}

	fakeFasitClient := FakeFasitClient{}
	appConfig := NaisAppConfig{FasitResources: FasitResources{Exposed: exposedResources}}

	t.Run("Calling updateFasit with resources returns no error", func(t *testing.T) {
		createApplicationInstanceCalled = false
		err := updateFasit(fakeFasitClient, deploymentRequest, usedResources, appConfig, hostname)
		assert.NoError(t, err)
		assert.True(t, createApplicationInstanceCalled)
	})
	t.Run("Calling updateFasit without hostname when you have exposed resources fails", func(t *testing.T) {
		createApplicationInstanceCalled = false
		err := updateFasit(fakeFasitClient, deploymentRequest, usedResources, appConfig, "")
		assert.Error(t, err)
		assert.False(t, createApplicationInstanceCalled)
	})
	t.Run("Calling updateFasit without hostname when you have no exposed resources works", func(t *testing.T) {
		createApplicationInstanceCalled = false
		appConfig.FasitResources.Exposed = nil
		err := updateFasit(fakeFasitClient, deploymentRequest, usedResources, appConfig, "")
		assert.NoError(t, err)
		assert.True(t, createApplicationInstanceCalled)
	})
}


func TestBuildingFasitPayloads(t *testing.T) {
	application := "appName"
	environment := "t1000"
	version := "2.1"
	exposedResourceIds := []int{1, 2, 3}
	usedResourceIds := []int{4, 5, 6}
	zone := "fss"
	alias := "resourceAlias"
	path := "/myPath"
	hostname := "hostname"
	wsdlGroupId := "myGroup"
	wsdlArtifactId := "myArtifactId"
	securityToken := "LDAP"
	allZones := true
	description := "myDescription"
	wsdlPath := fmt.Sprintf("http://maven.adeo.no/nexus/service/local/artifact/maven/redirect?r=m2internal&g=%s&a=%s&v=%s&e=zip", wsdlGroupId, wsdlArtifactId, version)

	deploymentRequest := NaisDeploymentRequest{
		Application: application,
		Environment: environment,
		Version:     version,
	}

	restResource := ExposedResource{
		Alias:        alias,
		ResourceType: "RestService",
		Path:         path,
		Description:  description,
	}
	webserviceResource := ExposedResource{
		Alias:          alias,
		ResourceType:   "WebserviceEndpoint",
		Path:           path,
		WsdlGroupId:    wsdlGroupId,
		WsdlArtifactId: wsdlArtifactId,
		WsdlVersion:    version,
		SecurityToken:  securityToken,
		Description:    description,
	}

	t.Run("Building ApplicationInstancePayload", func(t *testing.T) {
		payload := buildApplicationInstancePayload(deploymentRequest, exposedResourceIds, usedResourceIds)
		assert.Equal(t, application, payload.Application)
		assert.Equal(t, environment, payload.Environment)
		assert.Equal(t, version, payload.Version)
		assert.Equal(t, exposedResourceIds, payload.ExposedResources)
		assert.Equal(t, usedResourceIds, payload.UsedResources)
	})
	t.Run("Building RestService ResourcePayload", func(t *testing.T) {
		payload := buildResourcePayload(restResource, environment, zone, hostname)
		assert.Equal(t, "RestService", payload.Type)
		assert.Equal(t, alias, payload.Alias)
		assert.Equal(t, "https://"+hostname+path, payload.Properties.Url)
		assert.Equal(t, description, payload.Properties.Description)
		assert.Equal(t, environment, payload.Scope.Environment)
		assert.Equal(t, zone, payload.Scope.Zone)
	})
	t.Run("Building WebserviceEndpoint ResourcePayload", func(t *testing.T) {
		payload := buildResourcePayload(webserviceResource, environment, zone, hostname)
		assert.Equal(t, "WebserviceEndpoint", payload.Type)
		assert.Equal(t, alias, payload.Alias)
		assert.Equal(t, wsdlPath, payload.Properties.WsdlUrl)
		assert.Equal(t, description, payload.Properties.Description)
		assert.Equal(t, environment, payload.Scope.Environment)
		assert.Equal(t, zone, payload.Scope.Zone)
	})
	t.Run("Building RestService ResourcePayload with AllZones returns wider scope", func(t *testing.T) {
		restResource.AllZones = allZones
		payload := buildResourcePayload(restResource, environment, zone, hostname)
		assert.Equal(t, environment, payload.Scope.Environment)
		assert.Empty(t, payload.Scope.Zone)
	})
}

func TestGetFasitEnvironment(t *testing.T) {
	namespace := "namespace"

	defer gock.Off()
	gock.New("https://fasit.local").
		Get("/api/v2/environments/" + namespace).
		Reply(200).
		BodyString("anything")

	fasit := FasitClient{"https://fasit.local", "", ""}
	t.Run("Returns an error if environment isn't found", func(t *testing.T) {
		err := fasit.GetFasitEnvironment("notExisting")
		assert.Error(t, err)
		assert.False(t, gock.IsDone())
	})
	t.Run("Returns no error if environment is found", func(t *testing.T) {
		err := fasit.GetFasitEnvironment(namespace)
		assert.NoError(t, err)
		assert.True(t, gock.IsDone())
	})

}
func TestGetFasitApplication(t *testing.T) {
	application := "appname"

	defer gock.Off()
	gock.New("https://fasit.local").
		Get("/api/v2/applications/" + application).
		Reply(200).
		BodyString("anything")

	fasit := FasitClient{"https://fasit.local", "", ""}

	t.Run("Returns err if application isn't found", func(t *testing.T) {
		err := fasit.GetFasitApplication("Nonexistant")
		assert.Error(t, err)
		assert.False(t, gock.IsDone())
	})

	t.Run("Returns no error if application is found", func(t *testing.T) {
		err := fasit.GetFasitApplication(application)
		assert.NoError(t, err)
		assert.True(t, gock.IsDone())
	})

}
func TestGettingListOfResources(t *testing.T) {

	alias := "alias1"
	alias2 := "alias2"
	alias3 := "alias3"
	alias4 := "alias4"

	resourceType := "datasource"
	environment := "environment"
	application := "application"
	zone := "zone"

	fasit := FasitClient{"https://fasit.local", "", ""}

	defer gock.Off()
	gock.New("https://fasit.local").
		Get("/api/v2/scopedresource").
		MatchParam("alias", alias).
		MatchParam("type", resourceType).
		MatchParam("environment", environment).
		MatchParam("application", application).
		MatchParam("zone", zone).
		Reply(200).File("testdata/fasitResponse.json")

	gock.New("https://fasit.local").
		Get("/api/v2/scopedresource").
		MatchParam("alias", alias2).
		MatchParam("type", resourceType).
		MatchParam("environment", environment).
		MatchParam("application", application).
		MatchParam("zone", zone).
		Reply(200).File("testdata/fasitResponse2.json")

	gock.New("https://fasit.local").
		Get("/api/v2/scopedresource").
		MatchParam("alias", alias3).
		MatchParam("type", resourceType).
		MatchParam("environment", environment).
		MatchParam("application", application).
		MatchParam("zone", zone).
		Reply(200).File("testdata/fasitResponse3.json")

	gock.New("https://fasit.local").
		Get("/api/v2/scopedresource").
		MatchParam("alias", alias4).
		MatchParam("type", "applicationproperties").
		MatchParam("environment", environment).
		MatchParam("application", application).
		MatchParam("zone", zone).
		Reply(200).File("testdata/fasitResponse4.json")

	resources := []ResourceRequest{}
	resources = append(resources, ResourceRequest{alias, resourceType})
	resources = append(resources, ResourceRequest{alias2, resourceType})
	resources = append(resources, ResourceRequest{alias3, resourceType})
	resources = append(resources, ResourceRequest{alias4, "applicationproperties"})

	resourcesReplies, err := fasit.GetScopedResources(resources, environment, application, zone)

	assert.NoError(t, err)
	assert.Equal(t, 4, len(resourcesReplies))
	assert.Equal(t, alias, resourcesReplies[0].name)
	assert.Equal(t, alias2, resourcesReplies[1].name)
	assert.Equal(t, alias3, resourcesReplies[2].name)
	assert.Equal(t, alias4, resourcesReplies[3].name)
	assert.Equal(t, 2, len(resourcesReplies[3].properties))
	assert.Equal(t, "value1", resourcesReplies[3].properties["key1"])
	assert.Equal(t, "dc=preprod,dc=local", resourcesReplies[3].properties["key2"])
}

func TestResourceWithArbitraryPropertyKeys(t *testing.T) {
	fasit := FasitClient{"https://fasit.local", "", ""}

	defer gock.Off()
	gock.New("https://fasit.local").
		Get("/api/v2/scopedresource").
		MatchParam("alias", "alias").
		Reply(200).File("testdata/fasitResponse-arbitrary-keys.json")

	resource, appError := fasit.getScopedResource(ResourceRequest{"alias", "DataSource"}, "dev", "app", "zone")
	assert.Nil(t, appError)

	assert.Equal(t, "1", resource.properties["a"])
	assert.Equal(t, "2", resource.properties["b"])
	assert.Equal(t, "3", resource.properties["c"])
}

func TestResolvingSecret(t *testing.T) {
	fasit := FasitClient{"https://fasit.local", "", ""}

	defer gock.Off()
	gock.New("https://fasit.local").
		Get("/api/v2/scopedresource").
		MatchParam("alias", "aliaset").
		Reply(200).File("testdata/response-with-secret.json")

	gock.New("https://fasit.adeo.no").
		Get("/api/v2/secrets/696969").
		HeaderPresent("Authorization").
		Reply(200).BodyString("hemmelig")

	resource, appError := fasit.getScopedResource(ResourceRequest{"aliaset", "DataSource"}, "dev", "app", "zone")

	assert.Nil(t, appError)

	assert.Equal(t, "1", resource.properties["a"])
	assert.Equal(t, "hemmelig", resource.secret["password"])
}

func TestResolveCertifcates(t *testing.T) {
	fasit := FasitClient{"https://fasit.local", "", ""}

	t.Run("Fetch certificate file for resources of type certificate", func(t *testing.T) {

		defer gock.Off()
		gock.New("https://fasit.local").
			Get("/api/v2/scopedresource").
			MatchParam("alias", "alias").
			Reply(200).File("testdata/fasitCertResponse.json")
		gock.New("https://fasit.adeo.no").
			Get("/api/v2/resources/3024713/file/keystore").
			Reply(200).Body(bytes.NewReader([]byte("Some binary format")))

		resource, appError := fasit.getScopedResource(ResourceRequest{"alias", "Certificate"}, "dev", "app", "zone")

		assert.Nil(t, appError)

		assert.Equal(t, "Some binary format", string(resource.certificates["srvvarseloppgave_cert_keystore"]))

	})

	t.Run("Ignore non certificate resources with files ", func(t *testing.T) {

		defer gock.Off()
		gock.New("https://fasit.local").
			Get("/api/v2/scopedresource").
			MatchParam("alias", "alias").
			Reply(200).File("testdata/fasitFilesNoCertifcateResponse.json").
			Done()

		resource, appError := fasit.getScopedResource(ResourceRequest{"alias", "Certificate"}, "dev", "app", "zone")

		assert.Nil(t, appError)

		assert.Equal(t, 0, len(resource.certificates))

	})

}

func TestParseLoadBalancerConfig(t *testing.T) {
	t.Run("Parse array of load balancer config correctly", func(t *testing.T) {
		b, _ := ioutil.ReadFile("testdata/fasitLbConfigResponse.json")
		result, err := parseLoadBalancerConfig(b)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(result))
		assert.Equal(t, "root", result["url.with.root"])
		assert.Equal(t, "", result["url.without.root"])

	})

	t.Run("Err if no loadbalancer config is found", func(t *testing.T) {
		_, err := parseLoadBalancerConfig([]byte(`["json1","json2"]`))
		assert.Error(t, err)
	})

	t.Run("Empty map if empty response", func(t *testing.T) {
		result, err := parseLoadBalancerConfig([]byte(`[]`))
		assert.NoError(t, err)
		assert.Empty(t, result)

	})
}

func TestParseFilesObject(t *testing.T) {

	t.Run("Parse filename and fileurl correctly", func(t *testing.T) {
		var jsonMap map[string]interface{}
		json.Unmarshal([]byte(`{
			"keystore": {
				"filename": "keystore",
				"ref": "https://file.url"
			}}`), &jsonMap)
		fileName, fileUrl, err := parseFilesObject(jsonMap)

		assert.NoError(t, err)
		assert.Equal(t, "keystore", fileName)
		assert.Equal(t, "https://file.url", fileUrl)

	})

	t.Run("Err if filename not found ", func(t *testing.T) {
		var jsonMap map[string]interface{}
		json.Unmarshal([]byte(`{
			"keystore": {
				"ref": "https://file.url"
			}}`), &jsonMap)
		_, _, err := parseFilesObject(jsonMap)

		assert.Error(t, err)
	})

	t.Run("Err if fileurl not found ", func(t *testing.T) {
		var jsonMap map[string]interface{}
		json.Unmarshal([]byte(`{
			"keystore": {
				"filename": "keystore",
			}}`), &jsonMap)
		_, _, err := parseFilesObject(jsonMap)

		assert.Error(t, err)
	})

}
