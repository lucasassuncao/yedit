package editor

import "github.com/lucasassuncao/yedit/theme"

func renderHeader(title, file string, dirty, readOnly bool, width int, th resolvedTheme) string {
	if readOnly {
		title += " (READ-ONLY MODE)"
	}
	info := file
	if dirty {
		info = file + " ● modified"
	}
	return theme.RenderHeaderWith(title, info, "", width, th.colors)
}
