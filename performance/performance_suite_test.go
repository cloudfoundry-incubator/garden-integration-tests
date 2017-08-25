package performance_test

import (
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/garden/client"
	"code.cloudfoundry.org/garden/client/connection"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"

	"testing"
)

var (
	gardenHost   string
	gardenPort   string
	gardenClient garden.Client
	container    garden.Container

	rootfs string
)

// We suspect that bosh powerdns lookups have a low success rate (less than
// 99%) and when it fails, we get an empty string IP address instead of an
// actual error.
// Therefore, we explicity look up the IP once at the start of the suite with
// retries to minimise flakes.
func resolveHost(host string) string {
	if net.ParseIP(host) != nil {
		return host
	}

	var ip net.IP
	Eventually(func() error {
		ips, err := net.LookupIP(host)
		if err != nil {
			return err
		}
		if len(ips) == 0 {
			return errors.New("0 IPs returned from DNS")
		}
		ip = ips[0]
		return nil
	}, time.Minute, time.Second*5).Should(Succeed())

	return ip.String()
}

var _ = SynchronizedBeforeSuite(func() []byte {
	host := os.Getenv("GARDEN_ADDRESS")
	if host == "" {
		host = "10.244.16.6"
	}
	return []byte(resolveHost(host))
}, func(data []byte) {
	gardenHost = string(data)
})

func TestPerformance(t *testing.T) {
	RegisterFailHandler(Fail)
	SetDefaultEventuallyTimeout(5 * time.Second)

	BeforeEach(func() {
		gardenPort = os.Getenv("GARDEN_PORT")
		if gardenPort == "" {
			gardenPort = "7777"
		}
		rootfs = "docker:///cfgarden/garden-busybox"
	})

	JustBeforeEach(func() {
		gardenClient = client.New(connection.New("tcp", fmt.Sprintf("%s:%s", gardenHost, gardenPort)))

		var err error
		container, err = gardenClient.Create(garden.ContainerSpec{
			RootFSPath: rootfs,
		})
		Expect(err).ToNot(HaveOccurred())
		stdout := gbytes.NewBuffer()
		stderr := gbytes.NewBuffer()

		process, err := container.Run(garden.ProcessSpec{
			User: "root",
			Path: "sh",
			Args: []string{"-c", "while true; do sleep 1; done"},
		}, garden.ProcessIO{
			Stdout: stdout,
			Stderr: stderr,
		})
		Expect(err).ToNot(HaveOccurred())
		go process.Wait()
	})

	AfterEach(func() {
		Expect(gardenClient.Destroy(container.Handle())).To(Succeed())
	})

	RunSpecs(t, "Performance Suite")
}
