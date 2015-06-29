package docker

import (
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	//	"github.com/evergreen-ci/evergreen/command"
	//	"github.com/evergreen-ci/evergreen/db/bsonutil"
	"github.com/evergreen-ci/evergreen/hostutil"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/fsouza/go-dockerclient"
	//	"github.com/mitchellh/mapstructure"
	//	"math"
	"math/rand"
	"strconv"
	"time"
)

const (
	DockerStatusRunning = iota
	DockerStatusPaused
	DockerStatusRestarting
	DockerStatusKilled
	DockerStatusUnknown

	ProviderName = "docker"
)

type DockerManager struct {
}

type Settings struct {
	HostIp        string
	ImageName     string
	User          string
	Port          int
	ContainerName string
}

var (
	HostIp        = "10.4.102.195"
	ImageName     = "eg_sshd"
	User          = "root"
	Port          = 2376
	ContainerName = "trial"
)

func generateClient(settings *Settings) (*docker.Client, error) {
	endpoint := fmt.Sprintf("tcp://%s:%s", settings.HostIp, strconv.Itoa(settings.Port))
	return docker.NewTLSClient(endpoint, "./certificates/cert.pem", "./certificates/key.pem", "./certificates/ca.pem") // TODO deal with this (settings?)
}

//Validate checks that the settings from the config file are sane.
func (self *Settings) Validate() error {
	if self.HostIp == "" {
		return fmt.Errorf("HostIp must not be blank")
	}

	if self.ImageName == "" {
		return fmt.Errorf("ImageName must not be blank")
	}

	if self.User == "" {
		return fmt.Errorf("User must not be blank")
	}

	if self.Port != 2376 {
		return fmt.Errorf("Port must be set to 2376")
	}

	return nil
}

func (_ *DockerManager) GetSettings() cloud.ProviderSettings {
	return &Settings{}
}

func tmpGetSettings() *Settings {
	return &Settings{
		HostIp,
		ImageName,
		User,
		Port, // TODO if this really is restricted to 2376, then should be a constant
		ContainerName,
	}
}

// retrieveOpenPortBinding returns the first exposed port for a give docker container
// TODO probably only need to return the port part?
func retrieveOpenPortBinding(containerPtr *docker.Container) (docker.PortBinding, error) {
	exposedPorts := containerPtr.Config.ExposedPorts
	for k, _ := range exposedPorts {
		ports := containerPtr.NetworkSettings.Ports
		portBindings := ports[k]
		if len(portBindings) > 0 {
			return portBindings[0], nil
		}
	}
	return docker.PortBinding{}, fmt.Errorf("No available ports")
}

// SpawnInstance creates and starts a new Docker container
func (dockerMgr *DockerManager) SpawnInstance(d *distro.Distro, owner string, userHost bool) (*host.Host, error) {
	var err error

	if d.Provider != ProviderName {
		return nil, fmt.Errorf("Can't spawn instance of %v for distro %v: provider is %v", ProviderName, d.Id, d.Provider)
	}

	// Settings
	dockerSettings := tmpGetSettings()
	//	if err := mapstructure.Decode(d.ProviderSettings, digoSettings); err != nil {
	//		return nil, fmt.Errorf("Error decoding params for distro %v: %v", d.Id, err)
	//	}

	if err := dockerSettings.Validate(); err != nil {
		return nil, fmt.Errorf("Invalid Docker settings in distro %v: %v", d.Id, err)
	}

	// Initialize client
	dockerClient, err := generateClient(dockerSettings)
	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, "Docker initialize client API call failed "+
			"for host '%s': %v", "FILLER", err)
		return nil, err
	}

	// Build container
	newContainer, err := dockerClient.CreateContainer(
		docker.CreateContainerOptions{
			Name: dockerSettings.ContainerName,
			Config: &docker.Config{
				Image: dockerSettings.ImageName,
			},
			HostConfig: &docker.HostConfig{
				PublishAllPorts: true,
			},
		},
	)
	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, "Docker create container API call failed "+
			"for host '%s': %v", "FILLER", err)
		return nil, err
	}

	// Start container
	err = dockerClient.StartContainer(newContainer.ID, nil)
	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, "Docker start container API call failed "+
			"for host '%s': %v", "FILLER", err)
		return nil, err
	}

	// Retrieve container details
	newContainer, err = dockerClient.InspectContainer(newContainer.ID)
	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, "Docker inspect container API call failed "+
			"for host '%s': %v", "FILLER", err)
		return nil, err
	}

	portBinding, err := retrieveOpenPortBinding(newContainer)
	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, "Error with docker container '%v': "+
			"%v", newContainer.ID, err)
		return nil, err
	}
	hostPort := portBinding.HostPort
	hostStr := fmt.Sprintf("%s:%s", dockerSettings.HostIp, hostPort)

	// Add host info to db
	instanceName := "container-" +
		fmt.Sprintf("%d", rand.New(rand.NewSource(time.Now().UnixNano())).Int())
	host := &host.Host{
		Id:               newContainer.ID,
		Host:             hostStr,
		User:             d.User,
		Tag:              instanceName,
		Distro:           *d,
		CreationTime:     newContainer.Created,
		Status:           evergreen.HostUninitialized,
		TerminationTime:  model.ZeroTime,
		TaskDispatchTime: model.ZeroTime,
		Provider:         ProviderName,
		StartedBy:        owner,
	}

	err = host.Insert()
	if err != nil {
		return nil, evergreen.Logger.Errorf(slogger.ERROR, "Failed to insert new "+
			"host '%v': %v", host.Id, err)
	}

	evergreen.Logger.Logf(slogger.DEBUG, "Successfully inserted new host '%v' "+
		"for distro '%v'", host.Id, d.Id)

	return host, nil
}

// getStatus is a helper function which returns the enum representation of the status
// contained in a container's state
func getStatus(s *docker.State) int {
	var ret int
	if s.Running {
		ret = DockerStatusRunning
	} else if s.Paused {
		ret = DockerStatusPaused
	} else if s.Restarting {
		ret = DockerStatusRestarting
	} else if s.OOMKilled {
		ret = DockerStatusKilled
	} else {
		ret = DockerStatusUnknown
	}

	return ret
}

// GetInstanceStatus returns a universal status code representing the state
// of a container.
func (dockerMgr *DockerManager) GetInstanceStatus(host *host.Host) (cloud.CloudStatus, error) {
	dockerClient, err := generateClient(tmpGetSettings())
	// TODO refactor this to be cleaner
	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, "Docker initialize client API call failed "+
			"for host '%s': %v", "FILLER", err)
		return cloud.StatusUnknown, err
	}

	container, err := dockerClient.InspectContainer(host.Id)
	if err != nil {
		return cloud.StatusUnknown, fmt.Errorf("Failed to get container info: %v", err)
	}

	switch getStatus(&container.State) {
	case DockerStatusRestarting:
		return cloud.StatusInitializing, nil
	case DockerStatusRunning:
		return cloud.StatusRunning, nil
	case DockerStatusPaused:
		return cloud.StatusStopped, nil
	case DockerStatusKilled:
		return cloud.StatusTerminated, nil
	default:
		return cloud.StatusUnknown, nil
	}
}

//GetDNSName gets the DNS hostname of a droplet by reading it directly from
//the Docker API
func (dockerMgr *DockerManager) GetDNSName(host *host.Host) (string, error) {
	dockerClient, err := generateClient(tmpGetSettings())
	// TODO refactor this to be cleaner
	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, "Docker initialize client API call failed "+
			"for host '%s': %v", "FILLER", err)
		return "", err
	}

	container, err := dockerClient.InspectContainer(host.Id)
	if err != nil {
		return "", err
	}
	return container.NetworkSettings.IPAddress, nil
}

//CanSpawn returns if a given cloud provider supports spawning a new host
//dynamically. Always returns true for Docker.
func (dockerMgr *DockerManager) CanSpawn() (bool, error) {
	return true, nil
}

//TerminateInstance destroys a container.
func (dockerMgr *DockerManager) TerminateInstance(host *host.Host) error {
	dockerClient, err := generateClient(tmpGetSettings())
	// TODO refactor this to be cleaner
	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, "Docker initialize client API call failed "+
			"for host '%s': %v", "FILLER", err)
		return err
	}

	err = dockerClient.StopContainer(host.Id, 5)
	if err != nil {
		return evergreen.Logger.Errorf(slogger.ERROR, "Failed to stop container '%v': %v", host.Id, err)
	}

	err = dockerClient.RemoveContainer(
		docker.RemoveContainerOptions{
			ID: host.Id,
		},
	)
	if err != nil {
		return evergreen.Logger.Errorf(slogger.ERROR, "Failed to remove container '%v': %v", host.Id, err)
	}

	return host.Terminate()
}

//Configure populates a DockerManager by reading relevant settings from the
//config object.
func (dockerMgr *DockerManager) Configure(settings *evergreen.Settings) error {
	// TODO is this just for mci_settings.yml stuff?
	return nil
}

//IsSSHReachable checks if a container appears to be reachable via SSH by
//attempting to contact the host directly.
func (dockerMgr *DockerManager) IsSSHReachable(host *host.Host, keyPath string) (bool, error) {
	sshOpts, err := dockerMgr.GetSSHOptions(host, keyPath)
	if err != nil {
		return false, err
	}
	return hostutil.CheckSSHResponse(host, sshOpts)
}

//IsUp checks the container's state by querying the Docker API and
//returns true if the host should be available to connect with SSH.
func (dockerMgr *DockerManager) IsUp(host *host.Host) (bool, error) {
	cloudStatus, err := dockerMgr.GetInstanceStatus(host)
	if err != nil {
		return false, err
	}
	if cloudStatus == cloud.StatusRunning {
		return true, nil
	}
	return false, nil
}

func (dockerMgr *DockerManager) OnUp(host *host.Host) error {
	// TODO idk what this for!!!!
	return nil
}

//GetSSHOptions returns an array of default SSH options for connecting to a
//container.
func (dockerMgr *DockerManager) GetSSHOptions(host *host.Host, keyPath string) ([]string, error) {
	if keyPath == "" {
		return []string{}, fmt.Errorf("No key specified for DigitalOcean host")
	}
	opts := []string{"-i", keyPath}
	for _, opt := range host.Distro.SSHOptions {
		opts = append(opts, "-o", opt)
	}
	return opts, nil
}

// TimeTilNextPayment returns the amount of time until the next payment is due
// for the host. For Docker this is not relevant.
func (dockerMgr *DockerManager) TimeTilNextPayment(host *host.Host) time.Duration {
	return time.Duration(0)
}
