//go:build !windows

package tray

type Options struct {
	Tooltip string
	OnOpen  func()
	OnQuit  func()
}

func Run(options Options) error {
	select {}
}

func Quit() {}
