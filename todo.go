package main

import (
	"fmt"
	"os"
	"io"
	"time"
	"github.com/pelletier/go-toml/v2"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

var git_repo *git.Repository
var git_worktree *git.Worktree
var git_authenticated bool

var config Config
var data Data

type Config struct {
	DataPath string
	Debug bool

	GitUsername string
	GitMail string
	GitToken string
}

type Data struct {
	Test string
}

type Command struct {
	name string

	// If the command is supposed to change a value
	is_value_command bool
	string_value *string
	bool_value *bool

	// If the command is a parent
	children []Command
	parent_action Action // Action that gets activated when no children are specified
	has_parent_action bool
}

type Action struct {
}

func new_parent_command(name string, children ...Command) Command {
	return Command{
		name: name,
		children: children,
	}
}

func new_string_command(name string, value *string) Command {
	return Command{
		name: name,
		is_value_command: true,
		string_value: value,
	}
}

func new_bool_command(name string, value *bool) Command {
	return Command{
		name: name,
		is_value_command: true,
		bool_value: value,
	}
}

func (command Command) process_command(args []string) {
	// If no argument was passed
	if len(args) == 0 {
		// TODO: Print usage
		print_error("Not enough arguments passed to \"" + command.name + "\"")
		return
	}

	// If command is parent
	if len(command.children) > 0 {
		for _, child := range command.children {
			if child.name == args[0] {
				child.process_command(args[1:])
				return
			}
		}

		// If no child matching the argument was found
		print_error("Unknown command \"" + args[0] + "\" for \"" + command.name + "\"")

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

func main() {
	config = read_config()

	git_active := true
	if !check_git(config.DataPath) || config.DataPath == "" {
		print_debug("No valid git repository in config")
		git_active = false
	}

	if git_active {
		data = read_data()
		pull_git()
	}

	config_cmd := new_parent_command(
		"config",
		new_string_command(
			"data_path",
			&config.DataPath,
		),
		new_bool_command(
			"debug",
			&config.Debug,
		),
		new_parent_command(
			"git",
			new_string_command(
				"username",
				&config.GitUsername,
			),
			new_string_command(
				"mail",
				&config.GitMail,
			),
			new_string_command(
				"token",
				&config.GitToken,
			),
		),
	)

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "config":
			config_cmd.process_command(os.Args[2:])
		}
	}

	if git_active {
		write_data()
		push_git()
	}

	write_config(config)
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
