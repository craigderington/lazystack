package ui

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/craigderington/lazystack/internal/k8s"
	"github.com/craigderington/lazystack/internal/systemd"
)

// CategoryType represents which left pane section is focused
type CategoryType int

const (
	NamespacesCategory CategoryType = iota
	DeploymentsCategory
	PodsCategory
	ServicesCategory
)

// RightPaneTab represents tabs in the right pane
type RightPaneTab int

const (
	LogsTab RightPaneTab = iota
	StatsTab
	EnvTab
	ConfigTab
	TopTab
	ExecTab
)

// Messages
type namespacesLoadedMsg struct {
	namespaces []string
	err        error
}

type deploymentsLoadedMsg struct {
	deployments []k8s.DeploymentInfo
	err         error
}

type podsLoadedMsg struct {
	pods []k8s.PodInfo
	err  error
}

type servicesLoadedMsg struct {
	services []k8s.ServiceInfo
	err      error
}

type podLogsLoadedMsg struct {
	logs string
	err  error
}

type podMetricsLoadedMsg struct {
	metrics *k8s.PodMetrics
	err     error
}

type podEnvVarsLoadedMsg struct {
	envVars *k8s.PodEnvVars
	err     error
}

type resourceYAMLLoadedMsg struct {
	yaml string
	err  error
}

type tickMsg time.Time

// Model represents the application
type Model struct {
	width  int
	height int
	ready  bool

	// Left pane - multiple lists stacked vertically
	activeCategory   CategoryType
	namespacesList   list.Model
	deploymentsList  list.Model
	podsList         list.Model
	servicesList     list.Model
	selectedResource string

	// Right pane
	activeTab      RightPaneTab
	logsViewport   viewport.Model
	statsViewport  viewport.Model
	envViewport    viewport.Model
	configViewport viewport.Model

	// K8s data
	k8sManager       *k8s.Manager
	k8sInitError     error
	currentNamespace string
	namespaces       []string
	deployments      []k8s.DeploymentInfo
	pods             []k8s.PodInfo
	services         []k8s.ServiceInfo
	currentMetrics   *k8s.PodMetrics
	currentEnvVars   *k8s.PodEnvVars
	currentYAML          string
	selectedResourceType string // "pod", "deployment", "service"
	activePortForwards   map[string]*k8s.PortForward // key: "podname:localport"

	// Systemd
	systemdManager     *systemd.Manager
	systemdInitError   error

	// State
	statusMessage string
	err           error

	// Confirmation dialog
	showConfirmDialog bool
	confirmAction     string // "delete-pod", "delete-deployment", "scale-up", "scale-down"
	confirmResource   string // Resource name to confirm action on
	confirmReplicas   int32  // Target replica count for scale actions

	// Help screen
	showHelp bool
}

// Custom compact delegate without pipe bars
type compactDelegate struct{}

func (d compactDelegate) Height() int                             { return 1 }
func (d compactDelegate) Spacing() int                            { return 0 }
func (d compactDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d compactDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	str := item.(interface{ Title() string }).Title()

	// Add description if it exists
	if descItem, ok := item.(interface{ Description() string }); ok {
		if desc := descItem.Description(); desc != "" {
			str = str + " " + desc
		}
	}

	// Truncate to fit within list width
	// Account for prefix: "> " (2 chars) or "  " (2 chars)
	maxWidth := m.Width() - 2
	strWidth := lipgloss.Width(str)
	if strWidth > maxWidth && maxWidth > 3 {
		// Truncate character by character until it fits
		for lipgloss.Width(str+"...") > maxWidth && len(str) > 0 {
			str = str[:len(str)-1]
		}
		str = str + "..."
	}

	// Simple highlighting for selected item
	if index == m.Index() {
		fmt.Fprint(w, lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true).Render("> "+str))
	} else {
		fmt.Fprint(w, "  "+str)
	}
}

// List items
type simpleItem struct{ name string }

func (i simpleItem) FilterValue() string { return i.name }
func (i simpleItem) Title() string       { return i.name }
func (i simpleItem) Description() string { return "" }

type namespaceItem struct{ name string }

func (i namespaceItem) FilterValue() string { return i.name }
func (i namespaceItem) Title() string       { return i.name }
func (i namespaceItem) Description() string { return "" }

type deploymentItem struct{ deployment k8s.DeploymentInfo }

func (i deploymentItem) FilterValue() string { return i.deployment.Name }
func (i deploymentItem) Title() string       { return i.deployment.Name }
func (i deploymentItem) Description() string {
	statusIcon := "●"
	if i.deployment.Available < i.deployment.Replicas {
		statusIcon = "○"
	}
	return fmt.Sprintf("%s Ready: %s | Up-to-date: %d", statusIcon, i.deployment.Ready, i.deployment.UpToDate)
}

type podItem struct{ pod k8s.PodInfo }

func (i podItem) FilterValue() string { return i.pod.Name }
func (i podItem) Title() string       { return i.pod.Name }
func (i podItem) Description() string {
	statusIcon := "●"
	if i.pod.Status != "Running" {
		statusIcon = "○"
	}
	return fmt.Sprintf("%s %s | %s", statusIcon, i.pod.Status, i.pod.Ready)
}

type serviceItem struct{ service k8s.ServiceInfo }

func (i serviceItem) FilterValue() string { return i.service.Name }
func (i serviceItem) Title() string       { return i.service.Name }
func (i serviceItem) Description() string {
	return fmt.Sprintf("%s | %s | %s", i.service.Type, i.service.ClusterIP, i.service.Ports)
}

// NewModel creates the model
func NewModel() Model {
	// Use custom compact delegate without pipe bars
	delegate := compactDelegate{}

	// Create separate lists for each section
	namespacesList := list.New([]list.Item{}, delegate, 0, 0)
	namespacesList.Title = ""
	namespacesList.SetShowStatusBar(false)
	namespacesList.SetFilteringEnabled(false)
	namespacesList.SetShowHelp(false)
	namespacesList.SetShowPagination(false)
	namespacesList.SetShowTitle(false)

	deploymentsList := list.New([]list.Item{}, delegate, 0, 0)
	deploymentsList.Title = ""
	deploymentsList.SetShowStatusBar(false)
	deploymentsList.SetFilteringEnabled(false)
	deploymentsList.SetShowHelp(false)
	deploymentsList.SetShowPagination(false)
	deploymentsList.SetShowTitle(false)

	podsList := list.New([]list.Item{}, delegate, 0, 0)
	podsList.Title = ""
	podsList.SetShowStatusBar(false)
	podsList.SetFilteringEnabled(false)
	podsList.SetShowHelp(false)
	podsList.SetShowPagination(false)
	podsList.SetShowTitle(false)

	servicesList := list.New([]list.Item{}, delegate, 0, 0)
	servicesList.Title = ""
	servicesList.SetShowStatusBar(false)
	servicesList.SetFilteringEnabled(false)
	servicesList.SetShowHelp(false)
	servicesList.SetShowPagination(false)
	servicesList.SetShowTitle(false)

	// Initialize managers
	k8sMgr, k8sErr := k8s.NewManager()
	systemdMgr, _ := systemd.NewManager() // Initialized but not used in UI

	// Build initialization status message (Kubernetes only)
	var statusMsg string
	if k8sErr != nil {
		statusMsg = fmt.Sprintf("⚠ K8s init failed: %v", k8sErr)
	} else {
		statusMsg = "✓ K8s connected"
	}

	return Model{
		activeCategory:     NamespacesCategory,
		namespacesList:     namespacesList,
		deploymentsList:    deploymentsList,
		podsList:           podsList,
		servicesList:       servicesList,
		k8sManager:         k8sMgr,
		k8sInitError:       k8sErr,
		systemdManager:     systemdMgr,
		systemdInitError:   nil, // Not tracking systemd errors in UI
		currentNamespace:   "default",
		activeTab:          LogsTab,
		namespaces:         []string{},
		deployments:        []k8s.DeploymentInfo{},
		pods:               []k8s.PodInfo{},
		statusMessage:      statusMsg,
		activePortForwards: make(map[string]*k8s.PortForward),
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.loadNamespaces(),
		m.loadDeployments(),
		m.loadPods(),
		m.loadServices(),
		tick(),
	)
}

func (m Model) loadNamespaces() tea.Cmd {
	return func() tea.Msg {
		if m.k8sManager == nil {
			return namespacesLoadedMsg{err: fmt.Errorf("k8s manager not initialized")}
		}
		namespaces, err := m.k8sManager.ListNamespaces()
		return namespacesLoadedMsg{namespaces: namespaces, err: err}
	}
}

func (m Model) loadDeployments() tea.Cmd {
	return func() tea.Msg {
		if m.k8sManager == nil {
			return deploymentsLoadedMsg{err: fmt.Errorf("k8s manager not initialized")}
		}
		deployments, err := m.k8sManager.ListDeployments()
		return deploymentsLoadedMsg{deployments: deployments, err: err}
	}
}

func (m Model) loadPods() tea.Cmd {
	return func() tea.Msg {
		if m.k8sManager == nil {
			return podsLoadedMsg{err: fmt.Errorf("k8s manager not initialized")}
		}
		pods, err := m.k8sManager.ListPods()
		return podsLoadedMsg{pods: pods, err: err}
	}
}

func (m Model) loadServices() tea.Cmd {
	return func() tea.Msg {
		if m.k8sManager == nil {
			return servicesLoadedMsg{err: fmt.Errorf("k8s manager not initialized")}
		}
		services, err := m.k8sManager.ListServices()
		return servicesLoadedMsg{services: services, err: err}
	}
}

func (m Model) loadPodLogs(podName string) tea.Cmd {
	return func() tea.Msg {
		if m.k8sManager == nil {
			return podLogsLoadedMsg{err: fmt.Errorf("k8s manager not initialized")}
		}
		logs, err := m.k8sManager.GetPodLogs(podName, 100)
		return podLogsLoadedMsg{logs: logs, err: err}
	}
}

func (m Model) loadPodMetrics(podName string) tea.Cmd {
	return func() tea.Msg {
		if m.k8sManager == nil {
			return podMetricsLoadedMsg{err: fmt.Errorf("k8s manager not initialized")}
		}
		metrics, err := m.k8sManager.GetPodMetrics(podName)
		return podMetricsLoadedMsg{metrics: metrics, err: err}
	}
}

func (m Model) loadPodEnvVars(podName string) tea.Cmd {
	return func() tea.Msg {
		if m.k8sManager == nil {
			return podEnvVarsLoadedMsg{err: fmt.Errorf("k8s manager not initialized")}
		}
		envVars, err := m.k8sManager.GetPodEnvVars(podName)
		return podEnvVarsLoadedMsg{envVars: envVars, err: err}
	}
}

func (m Model) loadResourceYAML() tea.Cmd {
	return func() tea.Msg {
		if m.k8sManager == nil {
			return resourceYAMLLoadedMsg{err: fmt.Errorf("k8s manager not initialized")}
		}

		var yaml string
		var err error

		switch m.selectedResourceType {
		case "pod":
			yaml, err = m.k8sManager.GetPodYAML(m.selectedResource)
		case "deployment":
			yaml, err = m.k8sManager.GetDeploymentYAML(m.selectedResource)
		case "service":
			yaml, err = m.k8sManager.GetServiceYAML(m.selectedResource)
		default:
			err = fmt.Errorf("unknown resource type: %s", m.selectedResourceType)
		}

		return resourceYAMLLoadedMsg{yaml: yaml, err: err}
	}
}

func tick() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle confirmation dialog keys first
		if m.showConfirmDialog {
			switch msg.String() {
			case "y", "Y":
				// User confirmed - perform the action
				m.showConfirmDialog = false
				return m.performConfirmedAction()
			case "n", "N", "esc", "q":
				// User cancelled
				m.showConfirmDialog = false
				m.statusMessage = "Action cancelled"
				return m, nil
			}
			// Ignore other keys when dialog is showing
			return m, nil
		}

		// Handle help screen
		if m.showHelp {
			switch msg.String() {
			case "?", "q", "esc":
				m.showHelp = false
				return m, nil
			}
			// Ignore other keys when help is showing
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c", "q":
			// Cleanup port forwards
			if m.k8sManager != nil {
				for _, pf := range m.activePortForwards {
					m.k8sManager.StopPortForward(pf)
				}
			}
			if m.systemdManager != nil {
				m.systemdManager.Close()
			}
			return m, tea.Quit

		// Toggle help screen
		case "?":
			m.showHelp = !m.showHelp
			return m, nil

		// Switch active category with tab
		case "tab":
			// Cycle forward through categories
			m.activeCategory = (m.activeCategory + 1) % 4
			return m, nil
		case "shift+tab":
			// Cycle backward through categories
			m.activeCategory = (m.activeCategory - 1 + 4) % 4
			return m, nil
		case "1":
			m.activeCategory = NamespacesCategory
			return m, nil
		case "2":
			m.activeCategory = DeploymentsCategory
			// Auto-select first deployment when switching to this category
			if selected := m.deploymentsList.SelectedItem(); selected != nil {
				if item, ok := selected.(deploymentItem); ok {
					m.selectedResource = item.deployment.Name
					m.selectedResourceType = "deployment"
					m.activeTab = ConfigTab
					return m, m.loadResourceYAML()
				}
			}
			return m, nil
		case "3":
			m.activeCategory = PodsCategory
			// Auto-select first pod and load logs when switching to this category
			if selected := m.podsList.SelectedItem(); selected != nil {
				if item, ok := selected.(podItem); ok {
					m.selectedResource = item.pod.Name
					m.selectedResourceType = "pod"
					m.activeTab = LogsTab
					return m, tea.Batch(
						m.loadPodLogs(item.pod.Name),
						m.loadPodMetrics(item.pod.Name),
						m.loadPodEnvVars(item.pod.Name),
						m.loadResourceYAML(),
					)
				}
			}
			return m, nil
		case "4":
			m.activeCategory = ServicesCategory
			// Auto-select first service when switching to this category
			if selected := m.servicesList.SelectedItem(); selected != nil {
				if item, ok := selected.(serviceItem); ok {
					m.selectedResource = item.service.Name
					m.selectedResourceType = "service"
					m.activeTab = ConfigTab
					return m, m.loadResourceYAML()
				}
			}
			return m, nil

		// Switch tabs
		case "l":
			m.activeTab = LogsTab
			return m, nil
		case "s":
			m.activeTab = StatsTab
			return m, nil
		case "e":
			m.activeTab = EnvTab
			return m, nil
		case "c":
			m.activeTab = ConfigTab
			return m, nil
		case "t":
			m.activeTab = TopTab
			return m, nil
		case "x":
			m.activeTab = ExecTab
			return m, nil

		case "r":
			m.statusMessage = "Refreshing..."
			return m, tea.Batch(m.loadNamespaces(), m.loadDeployments(), m.loadPods(), m.loadServices())

		case "enter":
			// Handle selection based on active category
			switch m.activeCategory {
			case NamespacesCategory:
				selected := m.namespacesList.SelectedItem()
				if item, ok := selected.(namespaceItem); ok {
					m.currentNamespace = item.name
					m.k8sManager.SetNamespace(item.name)
					m.statusMessage = fmt.Sprintf("Switched to namespace: %s", item.name)
					m.activeCategory = PodsCategory
					return m, tea.Batch(m.loadDeployments(), m.loadPods(), m.loadServices())
				}
			case DeploymentsCategory:
				selected := m.deploymentsList.SelectedItem()
				if item, ok := selected.(deploymentItem); ok {
					m.selectedResource = item.deployment.Name
					m.selectedResourceType = "deployment"
					m.statusMessage = fmt.Sprintf("Selected deployment: %s", item.deployment.Name)
					m.activeTab = ConfigTab
					return m, m.loadResourceYAML()
				}
			case PodsCategory:
				selected := m.podsList.SelectedItem()
				if item, ok := selected.(podItem); ok {
					m.selectedResource = item.pod.Name
					m.selectedResourceType = "pod"
					m.statusMessage = fmt.Sprintf("Selected: %s", item.pod.Name)
					m.activeTab = LogsTab
					return m, tea.Batch(
						m.loadPodLogs(item.pod.Name),
						m.loadPodMetrics(item.pod.Name),
						m.loadPodEnvVars(item.pod.Name),
						m.loadResourceYAML(),
					)
				}
			}
			return m, nil

		case "d":
			// Show confirmation dialog for delete
			if m.activeCategory == PodsCategory {
				selected := m.podsList.SelectedItem()
				if item, ok := selected.(podItem); ok {
					m.showConfirmDialog = true
					m.confirmAction = "delete-pod"
					m.confirmResource = item.pod.Name
					return m, nil
				}
			} else if m.activeCategory == DeploymentsCategory {
				selected := m.deploymentsList.SelectedItem()
				if item, ok := selected.(deploymentItem); ok {
					m.showConfirmDialog = true
					m.confirmAction = "delete-deployment"
					m.confirmResource = item.deployment.Name
					return m, nil
				}
			}
			return m, nil

		case "+", "=":
			// Scale deployment up - show confirmation
			if m.activeCategory == DeploymentsCategory {
				selected := m.deploymentsList.SelectedItem()
				if item, ok := selected.(deploymentItem); ok {
					m.showConfirmDialog = true
					m.confirmAction = "scale-up"
					m.confirmResource = item.deployment.Name
					m.confirmReplicas = item.deployment.Replicas + 1
				}
			}
			return m, nil

		case "-", "_":
			// Scale deployment down - show confirmation
			if m.activeCategory == DeploymentsCategory {
				selected := m.deploymentsList.SelectedItem()
				if item, ok := selected.(deploymentItem); ok {
					if item.deployment.Replicas > 0 {
						m.showConfirmDialog = true
						m.confirmAction = "scale-down"
						m.confirmResource = item.deployment.Name
						m.confirmReplicas = item.deployment.Replicas - 1
					}
				}
			}
			return m, nil

		case "p":
			// Start port forward for selected pod
			if m.activeCategory == PodsCategory {
				selected := m.podsList.SelectedItem()
				if item, ok := selected.(podItem); ok {
					if m.k8sManager != nil {
						// Use a default port mapping (8080:80 for web apps, or 8080:8080)
						localPort := "8080"
						remotePort := "80" // Common default, could make this configurable

						pf, err := m.k8sManager.StartPortForward(item.pod.Name, localPort, remotePort)
						if err != nil {
							m.statusMessage = fmt.Sprintf("Error starting port-forward: %v", err)
						} else {
							key := fmt.Sprintf("%s:%s", item.pod.Name, localPort)
							m.activePortForwards[key] = pf
							m.statusMessage = fmt.Sprintf("Port-forward started: localhost:%s -> %s:%s", localPort, item.pod.Name, remotePort)
						}
					}
				}
			}
			return m, nil

		case "P":
			// Stop all port forwards
			if m.k8sManager != nil {
				for key, pf := range m.activePortForwards {
					m.k8sManager.StopPortForward(pf)
					delete(m.activePortForwards, key)
				}
				m.statusMessage = "Stopped all port-forwards"
			}
			return m, nil
		}

	case namespacesLoadedMsg:
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Error loading namespaces: %v", msg.err)
		} else {
			m.namespaces = msg.namespaces
			items := make([]list.Item, len(msg.namespaces))
			for i, ns := range msg.namespaces {
				items[i] = namespaceItem{name: ns}
			}
			m.namespacesList.SetItems(items)
		}
		return m, nil

	case deploymentsLoadedMsg:
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Error loading deployments: %v", msg.err)
		} else {
			m.deployments = msg.deployments
			items := make([]list.Item, len(msg.deployments))
			for i, deploy := range msg.deployments {
				items[i] = deploymentItem{deployment: deploy}
			}
			m.deploymentsList.SetItems(items)
		}
		return m, nil

	case podsLoadedMsg:
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Error loading pods: %v", msg.err)
		} else {
			m.pods = msg.pods
			items := make([]list.Item, len(msg.pods))
			for i, pod := range msg.pods {
				items[i] = podItem{pod: pod}
			}
			m.podsList.SetItems(items)
			if m.statusMessage == "Refreshing..." {
				m.statusMessage = fmt.Sprintf("Loaded %d pods from %s", len(msg.pods), m.currentNamespace)
			}
		}
		return m, nil

	case servicesLoadedMsg:
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Error loading services: %v", msg.err)
		} else {
			m.services = msg.services
			items := make([]list.Item, len(msg.services))
			for i, svc := range msg.services {
				items[i] = serviceItem{service: svc}
			}
			m.servicesList.SetItems(items)
		}
		return m, nil

	case podLogsLoadedMsg:
		if msg.err != nil {
			m.logsViewport.SetContent(fmt.Sprintf("Error: %v", msg.err))
		} else {
			m.logsViewport.SetContent(msg.logs)
			m.logsViewport.GotoBottom()
		}
		return m, nil

	case podMetricsLoadedMsg:
		if msg.err != nil {
			// Metrics not available - ignore silently or store nil
			m.currentMetrics = nil
			m.statsViewport.SetContent(fmt.Sprintf("Metrics unavailable for %s\n\nNote: Metrics require metrics-server to be installed in the cluster.\nInstall with: kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml", m.selectedResource))
		} else {
			m.currentMetrics = msg.metrics
			m.statsViewport.SetContent(m.renderStats())
		}
		return m, nil

	case podEnvVarsLoadedMsg:
		if msg.err != nil {
			// Env vars not available - ignore silently or store nil
			m.currentEnvVars = nil
			m.envViewport.SetContent(fmt.Sprintf("Error loading environment variables: %v", msg.err))
		} else {
			m.currentEnvVars = msg.envVars
			m.envViewport.SetContent(m.renderEnv())
		}
		return m, nil

	case resourceYAMLLoadedMsg:
		if msg.err != nil {
			m.currentYAML = fmt.Sprintf("Error loading YAML: %v", msg.err)
			m.configViewport.SetContent(m.currentYAML)
		} else {
			m.currentYAML = msg.yaml
			m.configViewport.SetContent(m.renderConfig())
		}
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.loadDeployments(), m.loadPods(), m.loadServices(), tick())

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true

		leftPaneWidth := m.width / 3
		rightPaneWidth := m.width - leftPaneWidth - 4

		// Calculate section height to fit all 4 sections in terminal
		// Total available height for left pane
		totalAvailable := m.height - 6 // minus title, status, help
		// Each section has: top border (1) + list items (N) + bottom border (1)
		// So: 4 * (N + 2) = totalAvailable
		// Therefore: N = (totalAvailable / 4) - 2
		sectionHeight := (totalAvailable / 4) - 2
		// Minimum 3 items per section
		if sectionHeight < 3 {
			sectionHeight = 3
		}

		m.namespacesList.SetSize(leftPaneWidth-4, sectionHeight)
		m.deploymentsList.SetSize(leftPaneWidth-4, sectionHeight)
		m.podsList.SetSize(leftPaneWidth-4, sectionHeight)
		m.servicesList.SetSize(leftPaneWidth-4, sectionHeight)

		if !m.logsViewport.HighPerformanceRendering {
			m.logsViewport = viewport.New(rightPaneWidth-4, m.height-12)
			m.statsViewport = viewport.New(rightPaneWidth-4, m.height-12)
			m.envViewport = viewport.New(rightPaneWidth-4, m.height-12)
			m.configViewport = viewport.New(rightPaneWidth-4, m.height-12)
		} else {
			m.logsViewport.Width = rightPaneWidth - 4
			m.logsViewport.Height = m.height - 12
			m.statsViewport.Width = rightPaneWidth - 4
			m.statsViewport.Height = m.height - 12
			m.envViewport.Width = rightPaneWidth - 4
			m.envViewport.Height = m.height - 12
			m.configViewport.Width = rightPaneWidth - 4
			m.configViewport.Height = m.height - 12
		}

		return m, nil
	}

	// Update the active list based on focused category
	var cmd tea.Cmd
	switch m.activeCategory {
	case NamespacesCategory:
		m.namespacesList, cmd = m.namespacesList.Update(msg)
		// Auto-switch namespace as user navigates
		if selected := m.namespacesList.SelectedItem(); selected != nil {
			if item, ok := selected.(namespaceItem); ok {
				if m.currentNamespace != item.name {
					m.currentNamespace = item.name
					m.k8sManager.SetNamespace(item.name)
					m.statusMessage = fmt.Sprintf("Switched to namespace: %s", item.name)
					cmds = append(cmds, tea.Batch(m.loadDeployments(), m.loadPods(), m.loadServices()))
				}
			}
		}
	case DeploymentsCategory:
		m.deploymentsList, cmd = m.deploymentsList.Update(msg)
		// Auto-select deployment as user navigates
		if selected := m.deploymentsList.SelectedItem(); selected != nil {
			if item, ok := selected.(deploymentItem); ok {
				if m.selectedResource != item.deployment.Name {
					m.selectedResource = item.deployment.Name
					m.selectedResourceType = "deployment"
					m.statusMessage = fmt.Sprintf("Selected: %s", item.deployment.Name)
					m.activeTab = ConfigTab
					cmds = append(cmds, m.loadResourceYAML())
				}
			}
		}
	case PodsCategory:
		m.podsList, cmd = m.podsList.Update(msg)
		// Auto-select pod and load logs as user navigates
		if selected := m.podsList.SelectedItem(); selected != nil {
			if item, ok := selected.(podItem); ok {
				if m.selectedResource != item.pod.Name {
					m.selectedResource = item.pod.Name
					m.selectedResourceType = "pod"
					m.statusMessage = fmt.Sprintf("Selected: %s", item.pod.Name)
					m.activeTab = LogsTab
					cmds = append(cmds, m.loadPodLogs(item.pod.Name))
					cmds = append(cmds, m.loadPodMetrics(item.pod.Name))
					cmds = append(cmds, m.loadPodEnvVars(item.pod.Name))
					cmds = append(cmds, m.loadResourceYAML())
				}
			}
		}
	case ServicesCategory:
		m.servicesList, cmd = m.servicesList.Update(msg)
		// Auto-select service as user navigates
		if selected := m.servicesList.SelectedItem(); selected != nil {
			if item, ok := selected.(serviceItem); ok {
				if m.selectedResource != item.service.Name {
					m.selectedResource = item.service.Name
					m.selectedResourceType = "service"
					m.statusMessage = fmt.Sprintf("Selected: %s", item.service.Name)
					m.activeTab = ConfigTab
					cmds = append(cmds, m.loadResourceYAML())
				}
			}
		}
	}
	cmds = append(cmds, cmd)

	// Update active viewport
	switch m.activeTab {
	case LogsTab:
		m.logsViewport, cmd = m.logsViewport.Update(msg)
		cmds = append(cmds, cmd)
	case StatsTab:
		m.statsViewport, cmd = m.statsViewport.Update(msg)
		cmds = append(cmds, cmd)
	case EnvTab:
		m.envViewport, cmd = m.envViewport.Update(msg)
		cmds = append(cmds, cmd)
	case ConfigTab:
		m.configViewport, cmd = m.configViewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// performConfirmedAction executes the action after user confirmation
func (m Model) performConfirmedAction() (tea.Model, tea.Cmd) {
	switch m.confirmAction {
	case "delete-pod":
		if m.k8sManager == nil {
			m.statusMessage = "Error: k8s manager not initialized"
			return m, nil
		}
		err := m.k8sManager.DeletePod(m.confirmResource)
		if err != nil {
			m.statusMessage = fmt.Sprintf("Error deleting pod: %v", err)
			return m, nil
		}
		m.statusMessage = fmt.Sprintf("Deleted pod: %s", m.confirmResource)
		return m, m.loadPods()

	case "delete-deployment":
		if m.k8sManager == nil {
			m.statusMessage = "Error: k8s manager not initialized"
			return m, nil
		}
		err := m.k8sManager.DeleteDeployment(m.confirmResource)
		if err != nil {
			m.statusMessage = fmt.Sprintf("Error deleting deployment: %v", err)
			return m, nil
		}
		m.statusMessage = fmt.Sprintf("Deleted deployment: %s", m.confirmResource)
		return m, m.loadDeployments()

	case "scale-up", "scale-down":
		if m.k8sManager == nil {
			m.statusMessage = "Error: k8s manager not initialized"
			return m, nil
		}
		err := m.k8sManager.ScaleDeployment(m.confirmResource, m.confirmReplicas)
		if err != nil {
			m.statusMessage = fmt.Sprintf("Error scaling: %v", err)
			return m, nil
		}
		m.statusMessage = fmt.Sprintf("Scaled %s to %d replicas", m.confirmResource, m.confirmReplicas)
		return m, m.loadDeployments()

	default:
		m.statusMessage = "Unknown action"
		return m, nil
	}
}

// View renders the UI
func (m Model) View() string {
	if !m.ready {
		return "Initializing lazystack..."
	}

	leftPaneWidth := m.width / 3
	rightPaneWidth := m.width - leftPaneWidth - 4

	// Build left pane - stack all sections vertically
	leftPaneSections := []string{
		m.renderSection(m.namespacesList, "[1] Namespaces", m.activeCategory == NamespacesCategory),
		m.renderSection(m.deploymentsList, "[2] Deployments", m.activeCategory == DeploymentsCategory),
		m.renderSection(m.podsList, "[3] Pods", m.activeCategory == PodsCategory),
		m.renderSection(m.servicesList, "[4] Services", m.activeCategory == ServicesCategory),
	}

	leftPaneContent := lipgloss.JoinVertical(lipgloss.Left, leftPaneSections...)

	// Constrain left pane to available height
	leftPaneStyle := lipgloss.NewStyle().
		Width(leftPaneWidth).
		MaxHeight(m.height - 6)

	leftPane := leftPaneStyle.Render(leftPaneContent)

	// Right pane
	rightPaneStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Width(rightPaneWidth).
		Height(m.height - 6).
		Padding(1)

	rightContent := m.renderRightPane()
	rightPane := rightPaneStyle.Render(rightContent)

	// Main view
	mainView := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, " ", rightPane)

	// Title bar
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("6")).
		Bold(true).
		Padding(0, 1)

	title := titleStyle.Render("lazystack - Kubernetes TUI")

	// Status bar
	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("6")).
		Bold(true).
		Padding(0, 1)

	status := statusStyle.Render(fmt.Sprintf("Namespace: %s | %s", m.currentNamespace, m.statusMessage))

	// Help
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Padding(0, 1)

	helpText := "?: help • tab/shift-tab: cycle • 1-4: jump • l: logs • s: stats • e: env • c: config • +/-: scale • p: port-fwd • P: stop • d: delete • r: refresh • q: quit"
	// Truncate help text if too wide
	maxHelpWidth := m.width - 4 // Leave some margin
	if len(helpText) > maxHelpWidth {
		helpText = helpText[:maxHelpWidth-3] + "..."
	}
	help := helpStyle.Render(helpText)

	baseView := lipgloss.JoinVertical(lipgloss.Left, title, mainView, status, help)

	// Overlay help screen if showing
	if m.showHelp {
		return m.overlayHelp(baseView)
	}

	// Overlay confirmation dialog if showing
	if m.showConfirmDialog {
		return m.overlayConfirmDialog(baseView)
	}

	return baseView
}

// overlayHelp renders the help screen over the base view
func (m Model) overlayHelp(baseView string) string {
	helpContent := `LAZYSTACK - KEYBOARD SHORTCUTS

NAVIGATION
  tab / shift+tab    Cycle through sections
  1-4                Jump to section (1:Namespaces 2:Deployments 3:Pods 4:Services)
  j / down           Move down in list
  k / up             Move up in list

TABS (Right Panel)
  l                  Logs tab
  s                  Stats tab (resource metrics)
  e                  Environment variables tab
  c                  Config tab (YAML view)
  t                  Top tab (coming soon)
  x                  Exec tab (coming soon)

ACTIONS
  +                  Scale deployment up (increase replicas)
  -                  Scale deployment down (decrease replicas)
  p                  Start port-forward (pod → localhost:8080)
  P                  Stop all port-forwards
  d                  Delete selected resource (pod/deployment)
  r                  Refresh current view

GENERAL
  ?                  Toggle this help screen
  q / ctrl+c         Quit application
  esc                Close dialog/help

Press ? or q to close this help screen`

	// Help box style
	helpBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("6")). // Cyan border
		Padding(1, 2).
		Width(70).
		Align(lipgloss.Left)

	// Render help box
	help := helpBox.Render(helpContent)

	// Place help in the center of the screen
	helpWithPadding := lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		help,
	)

	// Layer help on top of base view
	return helpWithPadding
}

// overlayConfirmDialog renders a confirmation dialog over the base view
func (m Model) overlayConfirmDialog(baseView string) string {
	// Determine dialog message based on action
	var message string
	switch m.confirmAction {
	case "delete-pod":
		message = fmt.Sprintf("Delete pod '%s'?", m.confirmResource)
	case "delete-deployment":
		message = fmt.Sprintf("Delete deployment '%s'?", m.confirmResource)
	case "scale-up":
		message = fmt.Sprintf("Scale '%s' to %d replicas?", m.confirmResource, m.confirmReplicas)
	case "scale-down":
		message = fmt.Sprintf("Scale '%s' to %d replicas?", m.confirmResource, m.confirmReplicas)
	default:
		message = fmt.Sprintf("Confirm action on '%s'?", m.confirmResource)
	}

	// Choose color based on action type
	var borderColor, textColor lipgloss.Color
	if m.confirmAction == "scale-up" || m.confirmAction == "scale-down" {
		borderColor = lipgloss.Color("3")  // Yellow for scale actions
		textColor = lipgloss.Color("3")    // Yellow text
	} else {
		borderColor = lipgloss.Color("1")  // Red for delete actions
		textColor = lipgloss.Color("1")    // Red text
	}

	// Dialog box style
	dialogBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(1, 2).
		Width(50).
		Align(lipgloss.Center)

	// Message style
	messageStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(textColor).
		Align(lipgloss.Center)

	// Prompt style
	promptStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Align(lipgloss.Center)

	dialogContent := lipgloss.JoinVertical(
		lipgloss.Center,
		messageStyle.Render(message),
		"",
		promptStyle.Render("[Y] Yes  [N] No"),
	)

	dialog := dialogBox.Render(dialogContent)

	// Place dialog in the center of the screen
	dialogWithPadding := lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		dialog,
	)

	// Layer dialog on top of base view
	return dialogWithPadding
}

// renderSection renders a single left pane section with border and title
func (m Model) renderSection(sectionList list.Model, title string, isFocused bool) string {
	borderColor := lipgloss.Color("240")
	if isFocused {
		borderColor = lipgloss.Color("6")
	}

	// Get the list content
	listContent := sectionList.View()

	// Calculate width and build custom border with embedded title
	width := m.width/3 - 2
	titleLen := len(title)

	// Build top border with embedded title: ─── [4] Services ───
	leftPad := 2
	rightPad := width - leftPad - titleLen - 4 // -4 for corner chars and spacing
	if rightPad < 2 {
		rightPad = 2
	}

	topBorder := lipgloss.NewStyle().Foreground(borderColor).Render(
		"╭" + strings.Repeat("─", leftPad) + " " + title + " " + strings.Repeat("─", rightPad) + "╮",
	)

	// Wrap list content with side borders
	lines := strings.Split(listContent, "\n")
	var borderedLines []string
	borderedLines = append(borderedLines, topBorder)

	leftBorder := lipgloss.NewStyle().Foreground(borderColor).Render("│")
	rightBorder := lipgloss.NewStyle().Foreground(borderColor).Render("│")

	// Only include up to the list's height to prevent overflow
	maxLines := sectionList.Height()
	for i, line := range lines {
		if i >= maxLines {
			break // Don't exceed the list's configured height
		}
		// Pad line to width
		lineWidth := lipgloss.Width(line)
		padding := width - lineWidth - 2 // -2 for left and right borders
		if padding < 0 {
			padding = 0
		}
		borderedLines = append(borderedLines, leftBorder + line + strings.Repeat(" ", padding) + rightBorder)
	}

	// Bottom border
	bottomBorder := lipgloss.NewStyle().Foreground(borderColor).Render(
		"╰" + strings.Repeat("─", width-2) + "╯",
	)
	borderedLines = append(borderedLines, bottomBorder)

	return strings.Join(borderedLines, "\n")
}

func (m Model) renderRightPane() string {
	// Tab headers
	tabStyle := lipgloss.NewStyle().Padding(0, 1)
	activeTabStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("6")).
		Padding(0, 1)

	tabs := []string{"Logs", "Stats", "Env", "Config", "Top", "Exec"}
	var tabHeaders []string

	for i, tab := range tabs {
		if RightPaneTab(i) == m.activeTab {
			tabHeaders = append(tabHeaders, activeTabStyle.Render(tab))
		} else {
			tabHeaders = append(tabHeaders, tabStyle.Render(tab))
		}
	}

	header := lipgloss.JoinHorizontal(lipgloss.Top, tabHeaders...)

	// Content based on active tab
	var content string
	switch m.activeTab {
	case LogsTab:
		if m.selectedResource != "" {
			content = m.logsViewport.View()
		} else {
			content = "Select a pod to view logs"
		}
	case StatsTab:
		content = m.statsViewport.View()
	case EnvTab:
		if m.selectedResource != "" {
			content = m.envViewport.View()
		} else {
			content = "Select a pod to view environment variables"
		}
	case ConfigTab:
		if m.selectedResource != "" {
			content = m.configViewport.View()
		} else {
			content = "Select a resource to view YAML configuration"
		}
	case TopTab:
		content = "Top/Resource usage (coming soon)"
	case ExecTab:
		content = m.renderExec()
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, "", content)
}

func (m Model) renderStats() string {
	if m.selectedResource == "" {
		return "Select a pod to view stats"
	}

	if m.currentMetrics == nil {
		return fmt.Sprintf("Loading metrics for %s...\n\nNote: Metrics require metrics-server to be installed in the cluster.\nInstall with: kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml", m.selectedResource)
	}

	// Calculate percentage bars (assume some reasonable limits)
	// CPU: assume 1000m (1 core) as 100%
	cpuPercent := (m.currentMetrics.CPUUsageMillis * 100) / 1000
	if cpuPercent > 100 {
		cpuPercent = 100
	}

	// Memory: assume 1Gi as 100%
	memPercent := (m.currentMetrics.MemoryUsageBytes * 100) / (1024 * 1024 * 1024)
	if memPercent > 100 {
		memPercent = 100
	}

	// Create visual bars
	cpuBar := createBar(int(cpuPercent), 20)
	memBar := createBar(int(memPercent), 20)

	return fmt.Sprintf(`Resource Metrics for: %s

CPU Usage:    %s  %s (%d%%)
Memory Usage: %s  %s (%d%%)

Raw Values:
  CPU:    %s
  Memory: %s

Namespace: %s`,
		m.currentMetrics.Name,
		m.currentMetrics.CPUUsageFormatted,
		cpuBar,
		cpuPercent,
		m.currentMetrics.MemoryUsageFormatted,
		memBar,
		memPercent,
		m.currentMetrics.CPUUsageFormatted,
		m.currentMetrics.MemoryUsageFormatted,
		m.currentMetrics.Namespace,
	)
}

// createBar creates a visual progress bar
func createBar(percent int, width int) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	filled := (percent * width) / 100
	empty := width - filled

	bar := ""
	for i := 0; i < filled; i++ {
		bar += "█"
	}
	for i := 0; i < empty; i++ {
		bar += "░"
	}

	return bar
}

func (m Model) renderEnv() string {
	if m.selectedResource == "" {
		return "Select a pod to view environment variables"
	}

	if m.currentEnvVars == nil {
		return fmt.Sprintf("Loading environment variables for %s...", m.selectedResource)
	}

	if len(m.currentEnvVars.Containers) == 0 {
		return fmt.Sprintf("No environment variables found for %s", m.currentEnvVars.PodName)
	}

	var output string
	output += fmt.Sprintf("Environment Variables for: %s\n", m.currentEnvVars.PodName)
	output += fmt.Sprintf("Namespace: %s\n\n", m.currentEnvVars.Namespace)

	// Iterate through each container
	for containerName, envVars := range m.currentEnvVars.Containers {
		output += fmt.Sprintf("━━━ Container: %s ━━━\n\n", containerName)

		if len(envVars) == 0 {
			output += "  (no environment variables)\n\n"
			continue
		}

		for _, env := range envVars {
			// Show env var name
			output += fmt.Sprintf("  %s\n", env.Name)

			// Show value or source
			if env.Value != "" {
				output += fmt.Sprintf("    = %s\n", env.Value)
			}
			if env.ValueFrom != "" {
				output += fmt.Sprintf("    → %s\n", env.ValueFrom)
			}
			output += "\n"
		}
	}

	return output
}

func (m Model) renderConfig() string {
	if m.selectedResource == "" {
		return "Select a resource to view YAML configuration"
	}

	if m.currentYAML == "" {
		return fmt.Sprintf("Loading YAML for %s...", m.selectedResource)
	}

	// Display the YAML with a header
	header := fmt.Sprintf("YAML Configuration for %s: %s (Namespace: %s)\n\n",
		m.selectedResourceType, m.selectedResource, m.currentNamespace)

	return header + m.currentYAML
}

func (m Model) renderExec() string {
	if m.selectedResource == "" {
		return "Select a pod to exec into"
	}
	return fmt.Sprintf("Exec into: %s\n\n$ kubectl exec -it %s -n %s -- /bin/sh\n\n(Coming soon: direct shell access)", m.selectedResource, m.selectedResource, m.currentNamespace)
}
