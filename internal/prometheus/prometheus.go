package prometheus

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/starttoaster/proxmox-exporter/internal/logger"
	wrappedProxmox "github.com/starttoaster/proxmox-exporter/internal/proxmox"
)

// Collector contains all prometheus metric Descs
type Collector struct {
	// Statuses
	nodeUp      *prometheus.Desc
	guestUp     *prometheus.Desc
	nodeVersion *prometheus.Desc

	// CPU
	clusterCPUsTotal *prometheus.Desc
	clusterCPUsAlloc *prometheus.Desc
	nodeCPUsTotal    *prometheus.Desc
	nodeCPUsAlloc    *prometheus.Desc

	// Mem
	clusterMemTotal *prometheus.Desc
	clusterMemAlloc *prometheus.Desc
	nodeMemTotal    *prometheus.Desc
	nodeMemAlloc    *prometheus.Desc

	// Storage
	storageTotal *prometheus.Desc
	storageUsed  *prometheus.Desc

	// Disk
	diskSmartHealth *prometheus.Desc

	// Certificates
	daysUntilCertExpiry *prometheus.Desc
}

// NewCollector constructor function for Collector
func NewCollector() *Collector {
	return &Collector{
		// Status metrics
		nodeUp: prometheus.NewDesc(fqAddPrefix("node_up"),
			"Shows whether host nodes in a proxmox cluster are up. (0=down,1=up)",
			[]string{"node"},
			nil,
		),
		guestUp: prometheus.NewDesc(fqAddPrefix("guest_up"),
			"Shows whether VMs and LXCs in a proxmox cluster are up. (0=down,1=up)",
			[]string{"node", "type", "name", "vmid"},
			nil,
		),
		nodeVersion: prometheus.NewDesc(fqAddPrefix("node_version"),
			"Shows PVE manager node version information",
			[]string{"node", "version"},
			nil,
		),

		// CPU metrics
		clusterCPUsTotal: prometheus.NewDesc(fqAddPrefix("cluster_cpus_total"),
			"Total number of vCPU (cores/threads) for a cluster.",
			nil,
			nil,
		),
		clusterCPUsAlloc: prometheus.NewDesc(fqAddPrefix("cluster_cpus_allocated"),
			"Total number of vCPU (cores/threads) allocated to guests for a cluster.",
			nil,
			nil,
		),
		nodeCPUsTotal: prometheus.NewDesc(fqAddPrefix("node_cpus_total"),
			"Total number of vCPU (cores/threads) for a node.",
			[]string{"node"},
			nil,
		),
		nodeCPUsAlloc: prometheus.NewDesc(fqAddPrefix("node_cpus_allocated"),
			"Total number of vCPU (cores/threads) allocated to guests for a node.",
			[]string{"node"},
			nil,
		),

		// Mem metrics
		clusterMemTotal: prometheus.NewDesc(fqAddPrefix("cluster_memory_total_bytes"),
			"Total amount of memory in bytes for a cluster.",
			nil,
			nil,
		),
		clusterMemAlloc: prometheus.NewDesc(fqAddPrefix("cluster_memory_allocated_bytes"),
			"Total amount of memory allocated in bytes to guests for a cluster.",
			nil,
			nil,
		),
		nodeMemTotal: prometheus.NewDesc(fqAddPrefix("node_memory_total_bytes"),
			"Total amount of memory in bytes for a node.",
			[]string{"node"},
			nil,
		),
		nodeMemAlloc: prometheus.NewDesc(fqAddPrefix("node_memory_allocated_bytes"),
			"Total amount of memory allocated in bytes to guests for a node.",
			[]string{"node"},
			nil,
		),

		// Disk metrics
		storageTotal: prometheus.NewDesc(fqAddPrefix("node_storage_total_bytes"),
			"Total amount of storage available in a volume on a node by storage type.",
			[]string{"node", "storage", "type", "shared"},
			nil,
		),
		storageUsed: prometheus.NewDesc(fqAddPrefix("node_storage_used_bytes"),
			"Total amount of storage used in a volume on a node by storage type.",
			[]string{"node", "storage", "type", "shared"},
			nil,
		),

		// Disk metrics
		diskSmartHealth: prometheus.NewDesc(fqAddPrefix("node_disk_smart_status"),
			"Disk SMART health status. (0=FAIL/Unknown,1=PASSED)",
			[]string{"node", "devpath"},
			nil,
		),

		// Cert metrics
		daysUntilCertExpiry: prometheus.NewDesc(fqAddPrefix("node_days_until_cert_expiration"),
			"Number of days until a certificate in PVE expires. Can report 0 days on metric collection errors, check exporter logs.",
			[]string{"node", "subject"},
			nil,
		),
	}
}

// Describe contains all the prometheus descriptors for this metric collector
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	// Status metrics
	ch <- c.nodeUp
	ch <- c.guestUp

	// CPU metrics
	ch <- c.clusterCPUsTotal
	ch <- c.clusterCPUsAlloc
	ch <- c.nodeCPUsTotal
	ch <- c.nodeCPUsAlloc

	// Mem metrics
	ch <- c.clusterMemTotal
	ch <- c.clusterMemAlloc
	ch <- c.nodeMemTotal
	ch <- c.nodeMemAlloc

	// Storage metrics
	ch <- c.storageTotal
	ch <- c.storageUsed

	// Disk metrics
	ch <- c.diskSmartHealth

	// Cert metrics
	ch <- c.daysUntilCertExpiry
}

// Collect instructs the prometheus client how to collect the metrics for each descriptor
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	// Retrieve node statuses for the cluster
	nodes, err := wrappedProxmox.GetNodes()
	if err != nil {
		logger.Logger.Error(err.Error())
		return
	}

	// Cluster level metric variables (added to in each iteration of the loop below)
	clusterCPUs := 0
	clusterCPUsAlloc := 0
	clusterMem := 0
	clusterMemAlloc := 0

	// Make waitgroup and results channel for cluster level metrics
	var wg sync.WaitGroup
	resultChan := make(chan collectNodeResponse, len(nodes.Data))

	// Collect node metrics from each of the nodes
	for _, node := range nodes.Data {
		wg.Add(1)
		go c.collectNode(ch, node, resultChan, &wg)
	}

	// Close the result channel after all goroutines finish
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results from the channel
	for result := range resultChan {
		clusterCPUs += result.clusterCPUs
		clusterCPUsAlloc += result.clusterCPUsAlloc
		clusterMem += result.clusterMem
		clusterMemAlloc += result.clusterMemAlloc
	}

	// Collect cluster metrics
	ch <- prometheus.MustNewConstMetric(c.clusterCPUsTotal, prometheus.GaugeValue, float64(clusterCPUs))
	ch <- prometheus.MustNewConstMetric(c.clusterCPUsAlloc, prometheus.GaugeValue, float64(clusterCPUsAlloc))
	ch <- prometheus.MustNewConstMetric(c.clusterMemTotal, prometheus.GaugeValue, float64(clusterMem))
	ch <- prometheus.MustNewConstMetric(c.clusterMemAlloc, prometheus.GaugeValue, float64(clusterMemAlloc))
}
