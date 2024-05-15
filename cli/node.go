package main

import "fmt"

type nodeOptions struct {
	List ListCmd `cmd:"" json:"-"`
	Info InfoCmd `cmd:"" json:"-"`

	NodeExtendedCmds `json:"-"`
}

func (n nodeOptions) Validate() error {
	fmt.Println("running node validate")
	return nil
}

type ListCmd struct{}

func (l ListCmd) Run() error {
	fmt.Println("running list command")
	return nil
}

type InfoCmd struct {
	Id string `arg:"" required:""`
}

func (i InfoCmd) Run() error {
	fmt.Println("running list command")
	fmt.Println(i.Id)
	return nil
}
