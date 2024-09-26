package main

import "context"

func checkVer(globals *Globals) error {
	if globals.Check {
		return nil
	}
	if !globals.DisableUpgradeCheck {
		iVer, err := versionCheck()
		if err != nil {
			return err
		}
		if globals.AutoUpgrade {
			u := Upgrade{
				installVersion: iVer,
			}
			globals.Check = false
			return u.Run(context.Background(), globals)
		}
	}
	return nil
}
