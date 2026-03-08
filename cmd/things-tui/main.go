package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
	"math/big"
    "hash/crc32"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
	thingscloud "github.com/arthursoares/things-cloud-sdk"
	memory "github.com/arthursoares/things-cloud-sdk/state/memory"
)

// -- Copied from things-cli --

type WireRepeaterDetail struct {
	Day     *int `json:"dy,omitempty"`
	Weekday *int `json:"wd,omitempty"`
}

type WireRepeater struct {
	FirstScheduledAt   int64                `json:"ia"`
	RepeatCount        int                  `json:"rc"`
	FrequencyUnit      int                  `json:"fu"`
	FrequencyAmplitude int                  `json:"fa"`
	Details            []WireRepeaterDetail `json:"of"`
	LastScheduledAt    int64                `json:"ed"`
	Version            int                  `json:"rrv"`
	Type               int                  `json:"tp"`
	TimeShift          int                  `json:"ts"`
	StartReference     int64                `json:"sr"`
}

func todayMidnightUTC() int64 {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Unix()
}

type WireNote struct {
	TypeTag  string `json:"_t"`
	Checksum int64  `json:"ch"`
	Value    string `json:"v"`
	Type     int    `json:"t"`
}

type WireExtension struct {
	Sn      map[string]any `json:"sn"`
	TypeTag string         `json:"_t"`
}

type writeEnvelope struct {
	id      string
	action  int
	kind    string
	payload any
}

func (w writeEnvelope) UUID() string { return w.id }

func (w writeEnvelope) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		T int    `json:"t"`
		E string `json:"e"`
		P any    `json:"p"`
	}{w.action, w.kind, w.payload})
}

type TaskCreatePayload struct {
	Tp   int              `json:"tp"`
	Sr   *int64           `json:"sr"`
	Dds  *int64           `json:"dds"`
	Rt   []string         `json:"rt"`
	Rmd  *int64           `json:"rmd"`
	Ss   int              `json:"ss"`
	Tr   bool             `json:"tr"`
	Dl   []string         `json:"dl"`
	Icp  bool             `json:"icp"`
	St   int              `json:"st"`
	Ar   []string         `json:"ar"`
	Tt   string           `json:"tt"`
	Do   int              `json:"do"`
	Lai  *int64           `json:"lai"`
	Tir  *int64           `json:"tir"`
	Tg   []string         `json:"tg"`
	Agr  []string         `json:"agr"`
	Ix   int              `json:"ix"`
	Cd   float64          `json:"cd"`
	Lt   bool             `json:"lt"`
	Icc  int              `json:"icc"`
	Md   *float64         `json:"md"`
	Ti   int              `json:"ti"`
	Dd   *int64           `json:"dd"`
	Ato  *int             `json:"ato"`
	Nt   WireNote         `json:"nt"`
	Icsd *int64           `json:"icsd"`
	Pr   []string         `json:"pr"`
	Rp   *string          `json:"rp"`
	Acrd *int64           `json:"acrd"`
	Sp   *float64         `json:"sp"`
	Sb   int              `json:"sb"`
	Rr   *json.RawMessage `json:"rr"`
	Xx   WireExtension    `json:"xx"`
}

func emptyNote() WireNote {
	return WireNote{TypeTag: "tx", Checksum: 0, Value: "", Type: 1}
}

func noteChecksum(s string) int64 {
	return int64(crc32.ChecksumIEEE([]byte(s)))
}

func textNote(s string) WireNote {
	return WireNote{TypeTag: "tx", Checksum: noteChecksum(s), Value: s, Type: 1}
}

func defaultExtension() WireExtension {
	return WireExtension{Sn: map[string]any{}, TypeTag: "oo"}
}

func generateUUID() string {
	u := uuid.New()
	const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	n := new(big.Int).SetBytes(u[:])
	base := big.NewInt(58)
	mod := new(big.Int)
	var encoded []byte
	for n.Sign() > 0 {
		n.DivMod(n, base, mod)
		encoded = append(encoded, alphabet[mod.Int64()])
	}
	for i, j := 0, len(encoded)-1; i < j; i, j = i+1, j-1 {
		encoded[i], encoded[j] = encoded[j], encoded[i]
	}
	return string(encoded)
}

func nowTs() float64 {
	return float64(time.Now().UnixNano()) / 1e9
}

type taskUpdate struct {
	fields map[string]any
}

func newTaskUpdate() *taskUpdate {
	return &taskUpdate{fields: map[string]any{
		"md": nowTs(),
	}}
}
func (u *taskUpdate) Status(ss int) *taskUpdate {
	u.fields["ss"] = ss
	return u
}
func (u *taskUpdate) StopDate(ts float64) *taskUpdate {
	u.fields["sp"] = ts
	return u
}
func (u *taskUpdate) build() map[string]any {
	return u.fields
}

func newTaskCreatePayload(title string) TaskCreatePayload {
	return TaskCreatePayload{
		Tp:   0,
		Sr:   nil,
		Dds:  nil,
		Rt:   []string{},
		Rmd:  nil,
		Ss:   0,
		Tr:   false,
		Dl:   []string{},
		Icp:  false,
		St:   0, // inbox
		Ar:   []string{},
		Tt:   title,
		Do:   0,
		Lai:  nil,
		Tir:  nil,
		Tg:   []string{},
		Agr:  []string{},
		Ix:   0,
		Cd:   nowTs(),
		Lt:   false,
		Icc:  0,
		Md:   nil,
		Ti:   0,
		Dd:   nil,
		Ato:  nil,
		Nt:   emptyNote(),
		Icsd: nil,
		Pr:   []string{},
		Rp:   nil,
		Acrd: nil,
		Sp:   nil,
		Sb:   0,
		Rr:   nil,
		Xx:   defaultExtension(),
	}
}

// -- Bubbletea App --

var (
	docStyle           = lipgloss.NewStyle().Margin(1, 2)
	titleStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	statusMessageStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("105"))
)

type item struct {
	title       string
	desc        string
	uuid        string
	completed   bool
	isProject   bool
}

func (i item) Title() string {
	prefix := "[ ] "
	if i.completed {
		prefix = "[x] "
	}
	if i.isProject {
		prefix = "📂 "
	}
	return prefix + i.title
}
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

type model struct {
	list                  list.Model
	client                *thingscloud.Client
	history               *thingscloud.History
	state                 *memory.State
	textInput             textinput.Model
	isAddingTask          bool
	isAddingProject       bool
	isAddingArea          bool
	isAddingRepeating     bool
	isAddingTaskToProject bool
	statusMessage         string
}

func initThings() (*thingscloud.Client, *thingscloud.History, *memory.State) {
	username := os.Getenv("THINGS_USERNAME")
	password := os.Getenv("THINGS_PASSWORD")
	if username == "" || password == "" {
		fmt.Println("Error: THINGS_USERNAME and THINGS_PASSWORD environment variables are required.")
		os.Exit(1)
	}

	c := thingscloud.New(thingscloud.APIEndpoint, username, password)
	if _, err := c.Verify(); err != nil {
		fmt.Printf("Error verifying login: %v\n", err)
		os.Exit(1)
	}

	history, err := c.OwnHistory()
	if err != nil {
		fmt.Printf("Error getting history: %v\n", err)
		os.Exit(1)
	}
	if err := history.Sync(); err != nil {
		fmt.Printf("Error syncing history: %v\n", err)
		os.Exit(1)
	}

	var allItems []thingscloud.Item
	startIndex := 0
	for {
		items, hasMore, err := history.Items(thingscloud.ItemsOptions{StartIndex: startIndex})
		if err != nil {
			fmt.Printf("Error fetching items: %v\n", err)
			os.Exit(1)
		}
		allItems = append(allItems, items...)
		if !hasMore {
			break
		}
		startIndex = history.LoadedServerIndex
	}

	state := memory.NewState()
	state.Update(allItems...)
	return c, history, state
}

func getTasksAsItems(state *memory.State) []list.Item {
	var items []list.Item
	for _, task := range state.Tasks {
		if task.InTrash || task.Status == 3 {
			continue // skip trashed or completed
		}
		desc := ""
		if task.Note != "" {
			lines := strings.Split(task.Note, "\n")
			if len(lines) > 0 {
				desc = lines[0]
			}
		}
		items = append(items, item{
			title:     task.Title,
			desc:      desc,
			uuid:      task.UUID,
			completed: false,
			isProject: task.Type == thingscloud.TaskTypeProject,
		})
	}
	return items
}

func initialModel() model {
	_, history, state := initThings()

	items := getTasksAsItems(state)

	m := list.New(items, list.NewDefaultDelegate(), 0, 0)
	m.Title = "Things Cloud Tasks"
	
	m.AdditionalFullHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add task")),
			key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "add project")),
			key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "add task to project")),
			key.NewBinding(key.WithKeys("A"), key.WithHelp("A", "add area")),
			key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "add repeating task (daily)")),
		}
	}
	m.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("a", "p", "A", "r", "t"), key.WithHelp("a/p/A/r/t", "add items")),
		}
	}

	ti := textinput.New()
	ti.Placeholder = "New Task Title..."
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = 40

	return model{
		list:      m,
		history:   history,
		state:     state,
		textInput: ti,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.isAddingTask || m.isAddingProject || m.isAddingTaskToProject || m.isAddingArea || m.isAddingRepeating {
			switch msg.String() {
			case "enter":
				title := m.textInput.Value()
				if title != "" {
					uuid := generateUUID()
					
					var payload any
					var kind string
					var statusMsg string
					var newItem item
					
					if m.isAddingArea {
						payload = map[string]any{
							"tt": title,
							"ix": 0,
							"tg": []string{},
							"xx": defaultExtension(),
						}
						kind = "Area3"
						statusMsg = fmt.Sprintf("Added Area: %s", title)
						newItem = item{title: title, uuid: uuid, completed: false, isProject: false} // Area is not exactly a task, but adding it to list
					} else if m.isAddingRepeating {
						taskPayload := newTaskCreatePayload(title)
						now := todayMidnightUTC()
						rr := WireRepeater{
							Version: 4, Type: 0, TimeShift: 0, StartReference: now,
							FirstScheduledAt: now, LastScheduledAt: 64092211200, RepeatCount: 0,
							FrequencyAmplitude: 1, FrequencyUnit: 16, // 16 = daily
							Details: []WireRepeaterDetail{{Day: new(int)}}, // 0 for daily
						}
						*rr.Details[0].Day = 0
						rrJSON, _ := json.Marshal(rr)
						rawRR := json.RawMessage(rrJSON)
						taskPayload.Rr = &rawRR
						payload = taskPayload
						kind = "Task6"
						statusMsg = fmt.Sprintf("Added Daily Repeating Task: %s", title)
						newItem = item{title: title + " 🔁", uuid: uuid, completed: false}
					} else if m.isAddingTask {
						taskPayload := newTaskCreatePayload(title)
						payload = taskPayload
						kind = "Task6"
						statusMsg = fmt.Sprintf("Added Task: %s", title)
						newItem = item{title: title, uuid: uuid, completed: false}
					} else if m.isAddingProject {
						taskPayload := newTaskCreatePayload(title)
						taskPayload.Tp = 1 // Project type
						taskPayload.St = 1 // Anytime (projects are triaged)
						payload = taskPayload
						kind = "Task6"
						statusMsg = fmt.Sprintf("Added Project: %s", title)
						newItem = item{title: title, uuid: uuid, completed: false, isProject: true}
					} else if m.isAddingTaskToProject {
						taskPayload := newTaskCreatePayload(title)
						selected, _ := m.list.SelectedItem().(item)
						taskPayload.Pr = []string{selected.uuid} // Add to project
						taskPayload.St = 1 // Anytime (tasks in projects are triaged)
						payload = taskPayload
						kind = "Task6"
						statusMsg = fmt.Sprintf("Added Task to %s: %s", selected.title, title)
						newItem = item{title: title, uuid: uuid, completed: false}
					}
					
					env := writeEnvelope{id: uuid, action: 0, kind: kind, payload: payload}
					if err := m.history.Write(env); err == nil {
						m.list.InsertItem(0, newItem)
						m.statusMessage = statusMsg
					} else {
						m.statusMessage = fmt.Sprintf("Error adding: %v", err)
					}
				}
				m.isAddingTask = false
				m.isAddingProject = false
				m.isAddingTaskToProject = false
				m.isAddingArea = false
				m.isAddingRepeating = false
				m.textInput.Reset()
				return m, nil
			case "esc":
				m.isAddingTask = false
				m.isAddingProject = false
				m.isAddingTaskToProject = false
				m.isAddingArea = false
				m.isAddingRepeating = false
				m.textInput.Reset()
				return m, nil
			}
			
			var cmd tea.Cmd
			m.textInput, cmd = m.textInput.Update(msg)
			return m, cmd
		}
		if m.list.FilterState() == list.Filtering {
			break
		}

		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "enter", " ":
			// Complete task
			i, ok := m.list.SelectedItem().(item)
			if ok && !i.completed && !i.isProject {
				ts := nowTs()
				u := newTaskUpdate().Status(3).StopDate(ts)
				env := writeEnvelope{id: i.uuid, action: 1, kind: "Task6", payload: u.build()}
				if err := m.history.Write(env); err == nil {
					i.completed = true
					m.list.SetItem(m.list.Index(), i)
					m.statusMessage = fmt.Sprintf("Completed: %s", i.title)
				} else {
					m.statusMessage = fmt.Sprintf("Error completing: %v", err)
				}
			}
            return m, nil
		case "n", "a":
			m.isAddingTask = true
			m.textInput.Placeholder = "New Task Title..."
			return m, textinput.Blink
		case "p":
			m.isAddingProject = true
			m.textInput.Placeholder = "New Project Title..."
			return m, textinput.Blink
		case "A":
			m.isAddingArea = true
			m.textInput.Placeholder = "New Area Title..."
			return m, textinput.Blink
		case "r":
			m.isAddingRepeating = true
			m.textInput.Placeholder = "New Daily Task Title..."
			return m, textinput.Blink
		case "t":
			if i, ok := m.list.SelectedItem().(item); ok && i.isProject {
				m.isAddingTaskToProject = true
				m.textInput.Placeholder = fmt.Sprintf("New Task in %s...", i.title)
				return m, textinput.Blink
			} else {
				m.statusMessage = "Select a project to add a task to."
				return m, nil
			}
		}

	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v-3) // leave room for input/status
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) View() string {
	var s strings.Builder

	if m.isAddingTask || m.isAddingProject || m.isAddingTaskToProject || m.isAddingArea || m.isAddingRepeating {
		var title string
		if m.isAddingTask {
			title = "Add Task"
		} else if m.isAddingProject {
			title = "Add Project"
		} else if m.isAddingTaskToProject {
			title = "Add Task to Project"
		} else if m.isAddingArea {
			title = "Add Area"
		} else if m.isAddingRepeating {
			title = "Add Daily Repeating Task"
		}
		
		s.WriteString(titleStyle.Render(title))
		s.WriteString("\n\n")
		s.WriteString(m.textInput.View())
		s.WriteString("\n\n(esc to cancel)")
	} else {
		s.WriteString(m.list.View())
	}

	if m.statusMessage != "" {
		s.WriteString("\n\n")
		s.WriteString(statusMessageStyle.Render(m.statusMessage))
	}

	return docStyle.Render(s.String())
}

func main() {
	fmt.Println("Loading state from Things Cloud...")
	m := initialModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v", err)
		os.Exit(1)
	}
}
