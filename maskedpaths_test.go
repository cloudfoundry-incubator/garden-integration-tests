package garden_integration_tests_test

import (
	"code.cloudfoundry.org/garden"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("MaskedPaths", func() {

	Context("when the container is unprivileged", func() {
		BeforeEach(func() {
			privilegedContainer = false
		})

		It("should mask the /proc/* files", func() {
			files := []string{
				"/proc/kcore",
				"/proc/sched_debug",
				"/proc/timer_stats",
				"/proc/timer_list",
			}
			for _, file := range files {
				out := gbytes.NewBuffer()
				process, err := container.Run(garden.ProcessSpec{
					Path: "ls",
					Args: []string{"-la", file},
				}, garden.ProcessIO{
					Stdout: out,
					Stderr: GinkgoWriter,
				})
				Expect(err).ToNot(HaveOccurred())

				Expect(process.Wait()).To(Equal(0))

				expectedFilePermissions := "crw-rw-rw-"
				expectedMajorVersion := "1,"

				Expect(out.Contents()).To(ContainSubstring(expectedFilePermissions), "file %v has wrong permissions", file)
				Expect(out.Contents()).To(ContainSubstring(expectedMajorVersion), "file %v has wrong permissions", file)
			}
		})

		It("should mask the /proc/* dirs", func() {
			dirs := []string{
				"/proc/scsi",
			}
			for _, dir := range dirs {
				out := gbytes.NewBuffer()
				process, err := container.Run(garden.ProcessSpec{
					Path: "ls",
					Args: []string{"-A", dir},
				}, garden.ProcessIO{
					Stdout: out,
					Stderr: GinkgoWriter,
				})
				Expect(err).ToNot(HaveOccurred())

				Expect(process.Wait()).To(Equal(0))

				Expect(out.Contents()).To(BeEmpty(), "directory %v is not empty", dir)
			}
		})

	})

})
