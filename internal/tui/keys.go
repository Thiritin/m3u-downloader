package tui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Up, Down, Left, Right, Enter, Back, Space key.Binding
	Filter, Refresh, ShowQueue, ShowBrowse    key.Binding
	Help, Quit                                key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Up:         key.NewBinding(key.WithKeys("up", "k")),
		Down:       key.NewBinding(key.WithKeys("down", "j")),
		Left:       key.NewBinding(key.WithKeys("left", "h")),
		Right:      key.NewBinding(key.WithKeys("right", "l")),
		Enter:      key.NewBinding(key.WithKeys("enter")),
		Back:       key.NewBinding(key.WithKeys("esc", "backspace")),
		Space:      key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "queue")),
		Filter:     key.NewBinding(key.WithKeys("/")),
		Refresh:    key.NewBinding(key.WithKeys("r")),
		ShowQueue:  key.NewBinding(key.WithKeys("q")),
		ShowBrowse: key.NewBinding(key.WithKeys("b")),
		Help:       key.NewBinding(key.WithKeys("?")),
		Quit:       key.NewBinding(key.WithKeys("ctrl+c")),
	}
}
