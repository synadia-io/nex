package main

import (
	"errors"
	"log/slog"
)

type configCmd struct {
	Conf    string `default:"all" enum:"all,global,node,run,devrun,stop,monitor,rootfs,lame,upgrade" help:"View current config"`
	Outfile string `help:"Save final config to file" placeholder:"nex.json"`
}

func (c configCmd) Run(ctx Context) error {
	var errs error
	if c.Conf == "all" || c.Conf == "global" {
		err := ctx.config.Global.Table()
		if err != nil {
			errs = errors.Join(errs, err)
			ctx.logger.Error("Failed to create table for global config", slog.Any("err", err))
		}
	}
	if c.Conf == "all" || c.Conf == "node" {
		err := ctx.config.Node.NodeExtendedCmds.Table()
		if err != nil {
			errs = errors.Join(errs, err)
			ctx.logger.Error("Failed to create table for node config", slog.Any("err", err))
		}
	}
	if c.Conf == "all" || c.Conf == "run" {
		err := ctx.config.Run.Table()
		if err != nil {
			errs = errors.Join(errs, err)
			ctx.logger.Error("Failed to create table for run config", slog.Any("err", err))
		}
	}
	if c.Conf == "all" || c.Conf == "devrun" {
		err := ctx.config.Devrun.Table()
		if err != nil {
			errs = errors.Join(errs, err)
			ctx.logger.Error("Failed to create table for devrun config", slog.Any("err", err))
		}
	}
	if c.Conf == "all" || c.Conf == "stop" {
		err := ctx.config.Stop.Table()
		if err != nil {
			errs = errors.Join(errs, err)
			ctx.logger.Error("Failed to create table for stop config", slog.Any("err", err))
		}
	}
	if c.Conf == "all" || c.Conf == "monitor" {
		err := ctx.config.Monitor.Table()
		if err != nil {
			errs = errors.Join(errs, err)
			ctx.logger.Error("Failed to create table for Monitor config", slog.Any("err", err))
		}
	}
	if c.Conf == "all" || c.Conf == "rootfs" {
		err := ctx.config.Rootfs.Table()
		if err != nil {
			errs = errors.Join(errs, err)
			ctx.logger.Error("Failed to create table for RootFS config", slog.Any("err", err))
		}
	}
	if c.Conf == "all" || c.Conf == "lame" {
		err := ctx.config.Lame.Table()
		if err != nil {
			errs = errors.Join(errs, err)
			ctx.logger.Error("Failed to create table for lame duck config", slog.Any("err", err))
		}
	}
	if c.Conf == "all" || c.Conf == "upgrade" {
		err := ctx.config.Upgrade.Table()
		if err != nil {
			errs = errors.Join(errs, err)
			ctx.logger.Error("Failed to create table for upgrade config", slog.Any("err", err))
		}
	}

	// TODO: Implement saving to file
	if c.Outfile != "" {
		ctx.logger.Warn("Saving to file not yet implemented.")
		return nil
	}

	return errs
}
