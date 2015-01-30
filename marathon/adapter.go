package marathon // import "github.com/CenturyLinkLabs/panamax-marathon-adapter/marathon"

import (
	"log"
	"fmt"

	"github.com/CenturyLinkLabs/gomarathon"
	"github.com/CenturyLinkLabs/panamax-marathon-adapter/api"
	"github.com/satori/go.uuid"
)

func newClient(endpoint string) *gomarathon.Client {
	url := endpoint
	if endpoint != "" {
		url = endpoint
	}
	log.Printf("Marathon Endpoint: %s", url)
	c, err := gomarathon.NewClient(url, nil)
	if err != nil {
		log.Fatal(err)
	}
	return c
}

type gomarathonClientAbstractor interface {
	ListApps() (*gomarathon.Response, error)
	GetApp(string) (*gomarathon.Response, error)
	GetAppTasks(string) (*gomarathon.Response, error)
	CreateApp(*gomarathon.Application) (*gomarathon.Response, error)
	CreateGroup(*gomarathon.Group) (*gomarathon.Response, error)
	DeleteApp(string) (*gomarathon.Response, error)
	DeleteGroup(string) (*gomarathon.Response, error)
}

type marathonAdapter struct {
	client gomarathonClientAbstractor
	conv   PanamaxServiceConverter
	generateUID func() string
}

func NewMarathonAdapter(endpoint string) *marathonAdapter {
	adapter := new(marathonAdapter)
	adapter.client = newClient(endpoint)
	adapter.conv = new(MarathonConverter)
	adapter.generateUID = func() string { return fmt.Sprintf("%s",uuid.NewV4()) }
	return adapter
}

func (m *marathonAdapter) GetServices() ([]*api.Service, *api.Error) {
	var apiErr *api.Error

	response, err := m.client.ListApps()
	if err != nil {
		apiErr = api.NewError(0, err.Error())
	}
	return m.conv.convertToServices(response.Apps), apiErr
}

func (m *marathonAdapter) GetService(id string) (*api.Service, *api.Error) {
	var apiErr *api.Error

	response, err := m.client.GetApp(m.sanitizeMarathonAppURL(id))
	if err != nil {
		apiErr = api.NewError(0, err.Error())
	}
	return m.conv.convertToService(response.App), apiErr
}

func (m *marathonAdapter) CreateServices(services []*api.Service) ([]*api.Service, *api.Error) {
	var apiErr *api.Error
	var deployments = make([]deployment, len(services))
	g := m.generateUID()

	dependents := m.findDependencies(services)
	for i := range services {
		if (dependents[services[i].Name] != 0) {
			services[i].Deployment.Count = 1
		}

		m.prepareServiceForDeployment(g, services[i])
		deployments[i] = createDeployment(services[i], m.client)
	}

	myGroup := new(deploymentGroup)
	myGroup.deployments = deployments

	status := deployGroup(myGroup, DEPLOY_TIMEOUT)

	switch status.code {
	case FAIL:
		apiErr = api.NewError(0, "Group deployment failed.")
	case TIMEOUT:
		apiErr = api.NewError(0, "Group deployment timed out.")
	}

	return services, apiErr
}

func (m *marathonAdapter) UpdateService(s *api.Service) *api.Error {
	return nil
}

func (m *marathonAdapter) DestroyService(id string) *api.Error {
	var apiErr *api.Error
	group, _ := splitServiceId(id, ".")

	_, err := m.client.DeleteApp(m.sanitizeMarathonAppURL(id))
	if err != nil {
		apiErr = api.NewError(0, err.Error())
	}

	m.client.DeleteGroup(group) // Remove group if possible we dont care about error or return.

	return apiErr
}

func (m *marathonAdapter) prepareServiceForDeployment(group string, service *api.Service) {
	var serviceName = service.Name

	service.Id = fmt.Sprintf("%s.%s", group, serviceName)
	service.Name = fmt.Sprintf("/%s/%s", group, serviceName)
	service.ActualState = "deployed"
}

func (m *marathonAdapter) sanitizeMarathonAppURL(id string) string {
	group, service := splitServiceId(id, ".")
	return fmt.Sprintf("%s/%s", group, service)
}

func (m *marathonAdapter) findDependencies(services []*api.Service) map[string]int {
	var deps = make(map[string]int)
	for s := range(services) {
		for l := range(services[s].Links) {
			deps[services[s].Links[l].Name] = 1
		}
	}

	return deps
}


