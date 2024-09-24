package main

import (
	"fmt"
	"os"
	"io"
	"time"
	"strconv"
	"unicode"
	"github.com/pelletier/go-toml/v2"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/gdamore/tcell"
)

var git_repo *git.Repository
var git_worktree *git.Worktree

var config Config
var data Data

var screen tcell.Screen
var default_style tcell.Style

var current_input_mode InputMode
var current_input_field_index int

var motion_log []Motion

// Goal types
type GoalType = int
const (
	TALLY = iota
	ELEMENTS = iota
)

type InputMode int
const (
	MOTIONS_MODE = iota
	PAGE_MODE = iota
)

type UIPageType int
const (
	HOME = iota
	GOALS = iota
	GOAL = iota
	NEW_GOAL = iota
)

type NumericMotionDataType int
const (
	GOAL_NUMERIC = iota
)

type NumericMotionData struct {
	data_type NumericMotionDataType
	value_entered int

	goals *[]Goal
}

func new_goal_numeric_motion_data(goals *[]Goal) NumericMotionData {
	return NumericMotionData{
		data_type: GOAL_NUMERIC,
		goals: goals,
	}
}

type MotionType int
const (
	MNEMONIC = iota
	NUMERIC = iota
)

type Motion struct {
	motion_type MotionType
	children []Motion
	page UIPage

	// Mnemonic
	key rune

	// Numeric
	numeric_data NumericMotionData
}

func new_mnemonic_motion(key_rune rune, children []Motion, page UIPage) Motion {
	return Motion{
		motion_type: MNEMONIC,
		children: children,
		page: page,
		key: key_rune,
	}
}

func new_numeric_motion(page UIPage, children []Motion, numeric_motion_data NumericMotionData) Motion {
	return Motion{
		motion_type: NUMERIC,
		children: children,
		page: page,
		numeric_data: numeric_motion_data,
	}
}

type UIPage struct {
	page_type UIPageType
	form Form
}

type Config struct {
	DataPath string
	Debug bool

	GitUsername string
	GitMail string
	GitToken string
}

type Data struct {
	Goals []Goal
	Tasks []Task
}

type Tally struct {
	Max int
	Progress int
}

type Element struct {
	Name string
	IsDone bool
}

type Goal struct {
	Name string

	GoalType GoalType
	Tally Tally
	Elements []Element
}

type InputType int
const (
	INT = iota
	STRING = iota
)

type InputField struct {
	name string
	field_type InputType

	int_value *int
	int_max int

	string_value *string
}

type Form interface {
	GetFields() *[]InputField
	Confirm()
}

type GoalForm struct {
	input_fields []InputField
	goal *Goal
}

func (goal_form GoalForm) GetFields() *[]InputField {
	return &goal_form.input_fields
}

func (goal_form GoalForm) Confirm() {
	data.Goals = append(data.Goals, *goal_form.goal)
	*goal_form.goal = Goal{}
}

func new_goal_form() GoalForm {
	goal:= &Goal{}

	return GoalForm{
		[]InputField{
			new_string_input_field("Name", &goal.Name),
			new_int_input_field("Type", &goal.GoalType, 1),
		},
		goal,
	}
}

func new_int_input_field(name string, value *int, max_value int) InputField {
	return InputField{
		name,
		INT,
		value,
		0,
		nil,
	}
}

func new_string_input_field(name string, value *string) InputField {
	return InputField{
		name,
		STRING,
		nil,
		0,
		value,
	}
}

type Task struct {
	Name string
}

func create_task(task_name string) {
	new_task := Task{
		Name: task_name,
	}

	data.Tasks = append(data.Tasks, new_task)
}

func main() {
	config = read_config()
	defer write_config(config)

	git_active := true
	if !check_git(config.DataPath) || config.DataPath == "" {
		print_debug("No valid git repository in config")
		git_active = false
	}

	if git_active {
		data = read_data()
		pull_git()

		defer push_git()
		defer write_data()
	}

	// Init tcell
	new_screen, err := tcell.NewScreen()
	if err != nil {
		panic(err)
	}
	screen = new_screen

	err = screen.Init()
	if err != nil {
		panic(err)
	}

	default_style = tcell.StyleDefault.Background(tcell.ColorDefault).Foreground(tcell.ColorDefault)
	screen.SetStyle(default_style)

	screen.Clear()

	// Set up pages
	home_page := UIPage{
		HOME,
		nil,
	}

	goals_page := UIPage{
		GOALS,
		nil,
	}

	goal_page := UIPage{
		GOAL,
		nil,
	}

	new_goal_page := UIPage{
		NEW_GOAL,
		new_goal_form(),
	}

	// Set up motions
	starting_motion := new_mnemonic_motion(
		':',
		[]Motion{
			new_mnemonic_motion(
				'g',
				[]Motion{
					new_numeric_motion(
						goal_page,
						[]Motion{},
						new_goal_numeric_motion_data(&data.Goals),
					),
					new_mnemonic_motion(
						'+',
						[]Motion{},
						new_goal_page,
					),
				},
				goals_page,
			),
		},
		home_page,
	)


	motion_log = []Motion{ starting_motion }

	// Game loop
	running := true
	for running {
		screen.Show()

		event := screen.PollEvent()
		switch event := event.(type) {
		case *tcell.EventResize:
			screen.Sync()
		case *tcell.EventKey:
			screen.Clear()
			if event.Key() == tcell.KeyCtrlC {
				running = false
			}

			switch current_input_mode {
			case MOTIONS_MODE:
				handle_motion_input(&motion_log, event)
			case PAGE_MODE:
				current_page := motion_log[len(motion_log) - 1].page
				handle_page_input(event, &current_page.form)
			}
		}

		if current_input_mode == MOTIONS_MODE {
			screen.ShowCursor(len(motion_log), 0)
		} else {
			screen.HideCursor()
		}

		last_motion := motion_log[len(motion_log) - 1]

		draw_motion_log(motion_log)
		draw_page(last_motion)
	}

	screen.Fini()
}

func draw_page(motion Motion) {
	// TODO: Delete this whole switch and just store all the "ui components" inside the pages (like how forms work)
	switch motion.page.page_type {
	case HOME:
		draw_home()
	case GOALS:
		draw_goals()
	case GOAL:
		draw_goal((*motion.numeric_data.goals)[motion.numeric_data.value_entered])
	case NEW_GOAL:
		draw_new_goal(*motion.page.form.GetFields())
	default:
		draw_text(0, 1, default_style, "No page")
	}
}

func draw_motion_log(motion_log []Motion) {
	col := 0
	for _, motion := range(motion_log) {
		switch motion.motion_type {
			case MNEMONIC:
				screen.SetContent(col, 0, motion.key, nil, default_style)
			case NUMERIC:
				screen.SetContent(col, 0, rune(motion.numeric_data.value_entered + '0'), nil, default_style)
		}

		col++
	}
}

func draw_home() {
	draw_text(0, 1, default_style, "Home")
	draw_text(0, 3, default_style, "Motions:")
	draw_text(0, 4, default_style, "g: all goals")
}

func draw_goals() {
	draw_text(0, 1, default_style, "Goals:")

	for index, goal := range(data.Goals) {
		draw_text(0, 1 + index, default_style, "%d: %s", index, goal.Name)
	}
}

func draw_goal(goal Goal) {
	draw_text(0, 1, default_style, "Name: %s", goal.Name)
}

func draw_new_goal(input_fields []InputField) {
	draw_input_fields(input_fields)
}

func draw_input_fields(input_fields []InputField) {
	for i, input_field := range(input_fields) {
		is_highlighted := false

		if current_input_field_index == i && current_input_mode == PAGE_MODE {
			is_highlighted = true
		}

		switch input_field.field_type {
		case INT:
			draw_text(0, 1 + i, default_style.Reverse(is_highlighted), "%s: %d", input_field.name, *input_field.int_value)
		case STRING:
			draw_text(0, 1 + i, default_style.Reverse(is_highlighted), "%s: %s", input_field.name, *input_field.string_value)
		}

		screen.SetStyle(default_style)
	}
}

func handle_motion_input(motion_log *[]Motion, key_event *tcell.EventKey) {
	// Handle switching focus
	if key_event.Key() == tcell.KeyEnter {
		current_input_mode = PAGE_MODE
		return
	}

	current_input_field_index = 0

	// Handle backspace
	if key_event.Key() == tcell.KeyBackspace {
		if len(*motion_log) == 1 {
			return
		}

		*motion_log = (*motion_log)[:len(*motion_log) - 1]
		return
	}

	// Handle motions
	last_motion := (*motion_log)[len(*motion_log) - 1]

	// Handle numeric motion
	num_value, err := strconv.Atoi(string(key_event.Rune()))
	// If the key pressed was a numeric
	if err == nil {
		// Look for numeric child
		var numeric_motion Motion
		found_numeric_motion := false
		for _, child := range(last_motion.children) {
			if child.motion_type == NUMERIC {
				numeric_motion = child
				found_numeric_motion = true
			}
		}

		if found_numeric_motion {
			if num_value >= len(*numeric_motion.numeric_data.goals) {
				return
			}

			switch numeric_motion.numeric_data.data_type {
				case GOAL_NUMERIC:
					numeric_motion.numeric_data.value_entered = num_value
					*motion_log = append(*motion_log, numeric_motion)
			}
		}

		return
	}

	// Look for mnemonic child
	key_rune := key_event.Rune()
	for _, child := range(last_motion.children) {
		if child.key == key_rune {
			*motion_log = append(*motion_log, child)
			return
		}
	}
}

func handle_page_input(key_event *tcell.EventKey, form *Form) {
	// Handle switching focus
	if key_event.Key() == tcell.KeyEscape {
		current_input_mode = MOTIONS_MODE
		return
	}

	if key_event.Key() == tcell.KeyUp && current_input_field_index == 0 {
		current_input_mode = MOTIONS_MODE
		return
	}

	if form == nil {
		return
	}

	input_fields := (*form).GetFields()

	if len(*input_fields) == 0 {
		return
	}

	// Handle confirming inputs
	if key_event.Key() == tcell.KeyEnter && current_input_field_index == len(*input_fields) - 1 {
		(*form).Confirm()

		// Exit page
		motion_log = motion_log[:len(motion_log) - 1]
		current_input_mode = MOTIONS_MODE

		return
	}

	// Handle switching between fields
	if key_event.Key() == tcell.KeyDown || key_event.Key() == tcell.KeyEnter {
		current_input_field_index = min(current_input_field_index + 1, len(*input_fields) - 1)
		return
	}

	if key_event.Key() == tcell.KeyUp {
		current_input_field_index = max(current_input_field_index - 1, 0)
		return
	}

	// Handle entering values into fields
	current_input_field := &(*input_fields)[current_input_field_index]
	switch current_input_field.field_type {
	case INT:
		input := key_event.Rune()
		if unicode.IsDigit(input) {
			*current_input_field.int_value *= 10
			value, _ := strconv.Atoi(string(input))
			*current_input_field.int_value += value
		}
	case STRING:
		input := key_event.Rune()
		if unicode.IsLetter(input) {
			*current_input_field.string_value += string(input)
		}
	}

	// TODO: Add the ability to erase
}

func draw_text(x int, y int, style tcell.Style, format string, args ...interface{}) {
	formatted_string := fmt.Sprintf(format, args...)

	for index, r := range[]rune(formatted_string) {
		screen.SetContent(x + index, y, r, nil, style)
	}
}

func read_config() Config {
	file, err := os.OpenFile("config.toml", os.O_CREATE, 0644)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	contents, err := io.ReadAll(file)
	if err != nil {
		panic(err)
	}

	var cfg Config
	err = toml.Unmarshal(contents, &cfg)
	if err != nil {
		panic(err)
	}

	return cfg
}

func write_config(config Config) {
	contents, err := toml.Marshal(config)
	if err != nil {
		panic(err)
	}

	file, err := os.OpenFile("config.toml", os.O_RDWR | os.O_CREATE | os.O_TRUNC, 0644)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	_, err = file.Write(contents)
	if err != nil {
		panic(err)
	}
}

func pull_git() {
	print_debug("Pulling from git")

	err := git_worktree.Pull(&git.PullOptions{
		RemoteName: "origin",
		Force: true,
	})

	if err == git.NoErrAlreadyUpToDate {
		print_debug("- Repo already up to date")
	} else if err != nil {
		panic(err)
	}
}

func push_git() {
	print_debug("Pushing to git")

	// If git has not been configure properly, don't try pushing
	if config.GitUsername == "" || config.GitMail == "" || config.GitToken == "" {
		print_debug("- Git isn't configured properly, aborting")
		return
	}

	status, err := git_worktree.Status()
	if err != nil {
		panic(err)
	}

	if status.IsClean() {
		print_debug("- No changes have been made, aborting")
		return
	}

	_, err = git_worktree.Add("data.toml")
	if err != nil {
		panic(err)
	}

	commit, err := git_worktree.Commit("Sync data", &git.CommitOptions{
		Author: &object.Signature{
			Name: config.GitUsername,
			Email: config.GitMail,
			When: time.Now(),
		},
	})

	if err != nil {
		panic(err)
	}

	print_debug("- Committed:", commit)

	err = git_repo.Push(&git.PushOptions{
		Auth: &http.BasicAuth{
			Username: config.GitUsername,
			Password: config.GitToken,
		},
	})

	if err != nil {
		panic(err)
	}

	print_debug("- Succesfully pushed to remote")
}

func read_data() Data {
	file, err := os.OpenFile(config.DataPath + "/data.toml", os.O_CREATE, 0644)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	contents, err := io.ReadAll(file)
	if err != nil {
		panic(err)
	}

	var data Data
	err = toml.Unmarshal(contents, &data)
	if err != nil {
		panic(err)
	}

	return data
}

func write_data() {
	contents, err := toml.Marshal(data)
	if err != nil {
		panic(err)
	}

	file, err := os.OpenFile(config.DataPath + "/data.toml", os.O_RDWR | os.O_CREATE | os.O_TRUNC, 0644)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	_, err = file.Write(contents)
	if err != nil {
		panic(err)
	}
}

func check_git(path string) bool {
	repo, err := git.PlainOpen(path)
	if err == git.ErrRepositoryNotExists {
		return false
	} else if err != nil {
		panic(err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		panic(err)
	}

	git_repo = repo
	git_worktree = worktree

	return true
}

// TODO: Remove this
func print_debug(message ...interface{}) {
}

func draw_debug_value(format string, args ...interface{}) {
	_, height := screen.Size()
	draw_text(0, height - 1, default_style, format, args...)
}
