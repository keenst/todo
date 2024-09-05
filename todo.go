package main

import (
	"fmt"
	"os"
	"io"
	"time"
	"bufio"
	"strconv"
	"github.com/pelletier/go-toml/v2"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

var git_repo *git.Repository
var git_worktree *git.Worktree

var config Config
var data Data

// Action types
type Action int
const (
	CreateTask = iota
	RemoveTask = iota
	CreateGoal = iota
	RemoveGoal = iota
	TallyMax = iota
)

// Goal types
type GoalType int
const (
	TallyGoal = iota
	ElementsGoal = iota
)

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

	GoalCounter int
	TaskCounter int
}

type Command struct {
	name string

	requires_arg bool

	// If the command is supposed to change a value
	is_value_command bool
	string_value *string
	bool_value *bool

	// If the command has an action tied to it
	is_action_command bool
	action Action

	// If the command is a parent
	children []Command

	// If this parent command passes a value to is child
	has_parent_value bool
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
	Index int

	GoalType GoalType
	Tally Tally
	Elements []Element
}

type Task struct {
	Name string
	Index int
}

func new_parent_command(name string, children ...Command) Command {
	return Command{
		name: name,
		children: children,
		requires_arg: true,
	}
}

func new_parent_value_command(name string, children ...Command) Command {
	return Command{
		name: name,
		children: children,
		requires_arg: true,
		has_parent_value: true,
	}
}

func new_string_value_command(name string, value *string) Command {
	return Command{
		name: name,
		is_value_command: true,
		requires_arg: true,
		string_value: value,
	}
}

func new_bool_value_command(name string, value *bool) Command {
	return Command{
		name: name,
		is_value_command: true,
		requires_arg: true,
		bool_value: value,
	}
}

func new_action_command(name string, action Action, requires_arg bool) Command {
	return Command{
		name: name,
		is_action_command: true,
		action: action,
		requires_arg: requires_arg,
	}
}

// parent_value: a value that might be passed down by parent
func (command Command) process_command(args []string, parent_value string) {
	// If no argument was passed
	if len(args) == 0 && command.requires_arg {
		// TODO: Print usage
		print_error("Not enough arguments passed to \"" + command.name + "\"")
		return
	}

	// If command is parent
	if len(command.children) > 0 {
		for _, child := range command.children {
			child_arg_offset := 0

			if child.has_parent_value {
				child_arg_offset = 1
				parent_value = args[0]
			}

			if child.name == args[child_arg_offset] {
				child.process_command(args[child_arg_offset + 1:], parent_value)
				return
			}
		}

		// If no child matching the argument was found
		print_error("Unknown command \"" + args[0] + "\" for \"" + command.name + "\"")
		return
	}

	// If command has action
	if len(args) == 0 {
		args = append(args, "")
	}

	if command.is_action_command{
		switch command.action {
		case CreateTask: create_task(args[0])
		case RemoveTask: remove_task(args[0])
		case CreateGoal: create_goal()
		case RemoveGoal: remove_goal(args[0])
		case TallyMax: tally_max(parent_value, args[0])
		}

		return
	}

	// If command is value
	if command.is_value_command {
		// String value
		if command.string_value != nil {
			*command.string_value = args[0]
		}

		// Bool value
		if command.bool_value != nil {
			if args[0] == "off" {
				*command.bool_value = false
			} else if args[0] == "on" {
				*command.bool_value = true
			} else {
				print_error("Value of \"" + command.name + "\" can only be either \"on\" or \"off\"")
			}
		}

		return
	}
}

func find_goal(index int) *Goal {
	for i := range len(data.Goals) {
		goal := &data.Goals[i]
		if goal.Index == index {
			return goal
		}
	}

	panic("Unable to find goal " + strconv.Itoa(index))
}

func tally_max(goal_index_str string, argument string) {
	goal_index, err := strconv.Atoi(goal_index_str)
	if err != nil {
		panic(err)
	}

	goal := find_goal(goal_index)

	if goal.GoalType != TallyGoal {
		fmt.Println("Goal", goal_index_str, "is not a tally goal")
	}

	if argument == "" {
		fmt.Println("Tally Max for goal " + goal_index_str + " is set to " + strconv.Itoa(goal.Tally.Max))
	} else {
		new_max, err := strconv.Atoi(argument)
		if err != nil {
			panic(err)
		}

		goal.Tally.Max = new_max
	}
}

func create_task(task_name string) {
	index := data.TaskCounter
	data.TaskCounter++

	new_task := Task{
		Name: task_name,
		Index: index,
	}

	data.Tasks = append(data.Tasks, new_task)
}

func remove_task(task_num string) {
	var index int
	var found_task bool
	for i, task := range(data.Tasks) {
		if strconv.Itoa(task.Index) == task_num {
			index = i
			found_task = true
			break
		}
	}

	if !found_task {
		print_error("Was unable to find task: \"" + task_num + "\"")
		return
	}

	data.Tasks = append(data.Tasks[:index], data.Tasks[index + 1:]...)
}

func create_goal() {
	name_input := InputVar{
		name: "Name",
		value_type: String,
	}

	type_input := InputVar{
		name: "Goal type (0: tally, 1: elements)",
		value_type: Int,
	}

	multivar_input(&name_input, &type_input)

	if type_input.int_value > 1 {
		print_error(strconv.Itoa(type_input.int_value) + " is not a valid value for Goal Type")
		return
	}

	index := data.GoalCounter
	data.GoalCounter++

	new_goal := Goal{
		Name: name_input.string_value,
		Index: index,
	}

	data.Goals = append(data.Goals, new_goal)
}

func remove_goal(goal_index string) {
	var index int
	var found_goal bool
	for i, goal := range(data.Goals) {
		if strconv.Itoa(goal.Index) == goal_index {
			index = i
			found_goal = true
			break
		}
	}

	if !found_goal {
		print_error("Was unable to find goal: \"" + goal_index + "\"")
		return
	}

	data.Goals = append(data.Goals[:index], data.Goals[index + 1:]...)
}

type ValueType int
const (
	String = iota
	Int = iota
)

type InputVar struct {
	name string
	value_type ValueType
	string_value string
	int_value int
}

// Prompts the user to enter multiple variables
func multivar_input(variables ...*InputVar) {
	reader := bufio.NewReader(os.Stdin)

	for _, variable := range variables {
		fmt.Print(variable.name + ": ")

		input, err := reader.ReadString('\n')
		if err != nil {
			panic(err)
		}

		switch variable.value_type {
		case String:
			(*variable).string_value = input[:len(input)-2]
		case Int:
			value, err := strconv.Atoi(input[:1])
			if err != nil {
				panic(err)
			}

			(*variable).int_value = value
		}
	}
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

	config_cmd := new_parent_command(
		"config",
		new_string_value_command(
			"data_path",
			&config.DataPath,
		),
		new_bool_value_command(
			"debug",
			&config.Debug,
		),
		new_parent_command(
			"git",
			new_string_value_command(
				"username",
				&config.GitUsername,
			),
			new_string_value_command(
				"mail",
				&config.GitMail,
			),
			new_string_value_command(
				"token",
				&config.GitToken,
			),
		),
	)

	task_cmd := new_parent_command(
		"task",
		new_action_command(
			"new",
			CreateTask,
			true,
		),
		new_action_command(
			"remove",
			RemoveTask,
			true,
		),
	)

	goal_cmd := new_parent_command(
		"goal",
		new_action_command(
			"new",
			CreateGoal,
			false,
		),
		new_action_command(
			"remove",
			RemoveGoal,
			true,
		),
		new_parent_value_command(
			"tally",
			new_action_command(
				"max",
				TallyMax,
				false,
			),
		),
	)

	if len(os.Args) == 1 {
		print_tasks()
		print_goals()
	} else if len(os.Args) > 1 {
		switch os.Args[1] {
		case "config":
			config_cmd.process_command(os.Args[2:], "")
		case "task":
			task_cmd.process_command(os.Args[2:], "")
		case "goal":
			goal_cmd.process_command(os.Args[2:], "")
		}
	}
}

func print_tasks() {
	fmt.Println("Tasks:")
	for _, task := range(data.Tasks) {
		fmt.Println(strconv.Itoa(task.Index) + ": " + task.Name)
	}
}

func print_goals() {
	fmt.Println("Goals:")
	for _, goal := range(data.Goals) {
		switch goal.GoalType {
		case ElementsGoal:
			var progress int
			for _, element := range(goal.Elements) {
				if element.IsDone {
					progress++
				}
			}

			fmt.Println(strconv.Itoa(goal.Index) + ": " + goal.Name + " [" + strconv.Itoa(progress) + "/" + strconv.Itoa(len(goal.Elements)) + "]")
		case TallyGoal:
			fmt.Println(strconv.Itoa(goal.Index) + ": " + goal.Name + " [" + strconv.Itoa(goal.Tally.Progress) + "/" + strconv.Itoa(goal.Tally.Max) + "]")
		}
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

func print_debug(args ...interface{}) {
	fmt.Printf("\u001b[90m")
	if config.Debug {
		fmt.Println(args...)
	}
	fmt.Printf("\u001b[37m")
}

func print_error(args ...interface{}) {
	fmt.Println(args...)
}
