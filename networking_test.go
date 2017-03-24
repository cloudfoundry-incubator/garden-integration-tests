package garden_integration_tests_test

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"code.cloudfoundry.org/garden"
	"github.com/eapache/go-resiliency/retrier"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Networking", func() {
	It("can be contacted after a NetIn", func() {
		_, err := container.Run(garden.ProcessSpec{
			Path: "sh",
			Args: []string{"-c", "echo hallo | nc -l -p 8080"},
			User: "root",
		}, garden.ProcessIO{
			Stdout: GinkgoWriter,
			Stderr: GinkgoWriter,
		})
		Expect(err).ToNot(HaveOccurred())

		hostPort, _, err := container.NetIn(0, 8080)
		Expect(err).ToNot(HaveOccurred())

		gardenHostname := strings.Split(gardenHost, ":")[0]
		Eventually(func() error {
			nc, err := gexec.Start(exec.Command("nc", gardenHostname, fmt.Sprintf("%d", hostPort)), GinkgoWriter, GinkgoWriter)
			if err != nil {
				Eventually(nc).Should(gbytes.Say("hallo"))
			}

			return err
		}).ShouldNot(HaveOccurred())
	})

	Describe("domain name resolution", func() {
		itCanResolve := func(domainName string) {
			defer func() {
				err := gardenClient.Destroy(container.Handle())
				Expect(err).NotTo(HaveOccurred())
			}()
			output := gbytes.NewBuffer()

			err := retrier.New(retrier.ConstantBackoff(30, 2*time.Second), nil).Run(func() error {
				proc, err := container.Run(garden.ProcessSpec{
					// We are using ping here rather than nslookup as we saw some
					// flakey behaviour with nslookup on our local concourse machines.
					// We're testing on the output of ping, which reports "bad address"
					// if it is unable to resolve a domain.
					Path: "ping",
					Args: []string{"-c", "1", domainName},
					User: "root",
				}, garden.ProcessIO{Stdout: output, Stderr: output})
				Expect(err).NotTo(HaveOccurred())

				_, err = proc.Wait()
				return err
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(output).ToNot(gbytes.Say("ping: bad address"))
		}

		It("can resolve localhost", func() {
			itCanResolve("localhost")
		})

		It("can resolve its hostname", func() {
			itCanResolve(container.Handle())
		})

		Context("when the rootFS contains /etc/resolv.conf", func() {
			BeforeEach(func() {
				rootfs = "docker:///debian#jessie"
			})

			It("can resolve domain names", func() {
				itCanResolve("www.example.com")
			})
		})

		Context("when the rootFS doesn't contain /etc/hosts or /etc/resolv.conf", func() {
			BeforeEach(func() {
				rootfs = "docker:///busybox:buildroot-2014.02"
			})

			It("can still resolve domain names because garden modifies /etc/resolv.conf", func() {
				itCanResolve("www.example.com")
			})

			It("can still resolve its hostname because garden modifies /etc/hosts", func() {
				itCanResolve(container.Handle())
			})
		})
	})

	Describe("subnet support", func() {
		BeforeEach(func() {
			networkSpec = fmt.Sprintf("192.168.%d.0/24", 12+GinkgoParallelNode())
		})

		Context("when destroying other containers on the same subnet", func() {
			It("should continue to route traffic successfully", func() {
				var (
					err            error
					googleDNSIP    string
					otherContainer garden.Container
				)

				googleDNSIP = "8.8.8.8"
				for i := 0; i < 5; i++ {
					otherContainer, err = gardenClient.Create(garden.ContainerSpec{
						Network: networkSpec,
					})
					Expect(err).NotTo(HaveOccurred())

					Expect(gardenClient.Destroy(otherContainer.Handle())).To(Succeed())
					err := checkConnection(container, googleDNSIP, 53)
					if err != nil {
						checkPing(container, googleDNSIP)
					}
					Expect(err).NotTo(HaveOccurred())
				}
			})
		})

		Context("when creating a container in a previously used subnet", func() {
			var newContainer garden.Container

			JustBeforeEach(func() {
				var err error

				Expect(gardenClient.Destroy(container.Handle())).To(Succeed())

				newContainer, err = gardenClient.Create(garden.ContainerSpec{
					Network: networkSpec,
				})
				Expect(err).NotTo(HaveOccurred())
			})
			AfterEach(func() {
				Expect(gardenClient.Destroy(newContainer.Handle())).To(Succeed())
			})

			It("should continue to route traffic successfully", func() {
				googleDNSIP := "8.8.8.8"
				Expect(checkConnection(newContainer, googleDNSIP, 53)).To(Succeed())
			})
		})
	})
})

func checkConnection(container garden.Container, ip string, port int) error {
	process, err := container.Run(garden.ProcessSpec{
		User: "root",
		Path: "sh",
		Args: []string{"-c", fmt.Sprintf("echo hello | nc -w1 %s %d", ip, port)},
	}, garden.ProcessIO{Stdout: GinkgoWriter, Stderr: GinkgoWriter})
	if err != nil {
		return err
	}

	exitCode, err := process.Wait()
	if err != nil {
		return err
	}

	if exitCode == 0 {
		return nil
	} else {
		return fmt.Errorf("Request failed. Process exited with code %d", exitCode)
	}
}

func checkPing(container garden.Container, ip string) error {
	p, err := container.Run(garden.ProcessSpec{
		User: "root",
		Path: "ping",
		Args: []string{"-c", "10", "-W", "1", ip},
	}, garden.ProcessIO{Stdout: GinkgoWriter, Stderr: GinkgoWriter})
	p.Wait()

	return err
}
