package main

import (
	"bufio"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"io"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// FileManager holds the state of the c3 file manager
type FileManager struct {
	app          *tview.Application
	list         *tview.List
	preview      *tview.TextView
	searchInput  *tview.InputField
	listFlex     *tview.Flex
	renameInput  *tview.InputField
	currentDir   string
	showHidden   bool
	searchQuery  string
	searchActive bool
	clipboardPath  string
	clipboardIsCut bool
	renameActive   bool
	confirmModal   *tview.Modal
	modalActive    bool
}

// NewFileManager initializes a new file manager
func NewFileManager() *FileManager {
	fm := &FileManager{
		app:          tview.NewApplication(),
		list:         tview.NewList().ShowSecondaryText(false),
		preview:      tview.NewTextView().SetText("Select a file or directory to preview").SetDynamicColors(true),
		searchInput:  tview.NewInputField().SetLabel("Search: "),
		listFlex:     tview.NewFlex().SetDirection(tview.FlexRow),
		renameInput:   tview.NewInputField().SetLabel("Rename to: "),
		currentDir:   getCurrentDir(),
		showHidden:   false, // Hide hidden files by default
		searchQuery:  "",
		searchActive: false,
		renameActive:  false,
		confirmModal:  tview.NewModal(),
		modalActive:   false,
	}
	// Initially only add list to listFlex
	fm.listFlex.AddItem(fm.list, 0, 1, true)
	return fm
}

// getCurrentDir returns the current working directory
func getCurrentDir() string {
	dir, err := os.Getwd()
	if err != nil {
		panic(err) // Simplify for Day 3; we'll handle errors later
	}
	return dir
}

// setupUI configures the TUI layout and starts the app
func (fm *FileManager) setupUI() {
	// Create main flex: listFlex on left, preview on right, modal on top when active
	flex := tview.NewFlex().
		AddItem(fm.listFlex, 0, 1, true).
		AddItem(fm.preview, 0, 1, false)
	fm.app.SetRoot(fm.listFlex, true) // Initially set root to listFlex

	// Style the list
	fm.list.SetBorder(true).SetTitle("c3 - File Manager").SetTitleAlign(tview.AlignLeft)

	// Populate list with files
	fm.updateFileList()

	// Set key bindings
	fm.setupKeyBindings()

	// Run the app
	if err := fm.app.SetRoot(flex, true).SetFocus(fm.list).Run(); err != nil {
		panic(err)
	}
}

// updateFileList populates the list with files and directories
func (fm *FileManager) updateFileList() {
	fm.list.Clear()
	dirEntries, err := os.ReadDir(fm.currentDir)
	if err != nil {
		fm.list.AddItem("[red]Error reading directory[-]", err.Error(), 0, nil)
		fm.preview.SetText("[red]Error: " + err.Error() + "[-]")
		return
	}

	// Filter and sort entries: directories first, then files
	var filteredEntries, dirs, files []os.DirEntry
	for _, entry := range dirEntries {
		if !fm.showHidden && strings.HasPrefix(entry.Name(), ".") {
			continue // Skip hidden files if showHidden is false
		}
		if fm.searchQuery != "" && !strings.HasPrefix(strings.ToLower(entry.Name()), strings.ToLower(fm.searchQuery)) {
			continue // Skip entries not starting with search query
		}
		if entry.IsDir() {
			dirs = append(dirs, entry)
		} else {
			files = append(files, entry)
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name() < dirs[j].Name() })
	sort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })

	// Build filteredEntries to match List order (dirs + files)
	filteredEntries = append(filteredEntries, dirs...)
	filteredEntries = append(filteredEntries, files...)

	// Add directories to List
	for _, dir := range dirs {
		dirName := dir.Name() // Capture for closure
		fm.list.AddItem("[blue]"+dirName+"/[-]", "", 0, func() {
			fm.navigateTo(dirName)
		})
	}

	// Add files to List
	for _, file := range files {
		fm.list.AddItem("[white]"+file.Name()+"[-]", "", 0, nil) // No action for files
	}

	// Update list title with abbreviated path (last 2 directories)
	promptPath := fm.currentDir
	parts := strings.Split(promptPath, string(os.PathSeparator))
	if len(parts) > 2 {
		promptPath = strings.Join(parts[len(parts)-2:], "/")
	} else if strings.HasPrefix(promptPath, "/") {
		promptPath = promptPath[1:] // Remove leading slash for short paths
	}
	fm.list.SetTitle("c3 - " + promptPath)

	// Update preview with current directory, hidden status, and search query
	hiddenStatus := "Hidden files: " + map[bool]string{true: "shown", false: "hidden"}[fm.showHidden]
	searchStatus := "Search: " + map[string]string{"": "none", fm.searchQuery: fm.searchQuery}[fm.searchQuery]
	fm.preview.SetText("Current directory: " + fm.currentDir + "\n" + hiddenStatus + "\n" + searchStatus)

	// Update preview when selection changes
	fm.list.SetChangedFunc(func(index int, mainText, secondaryText string, shortcut rune) {
		fm.updatePreview(index, filteredEntries)
	})

	// Trigger preview update for initial selection
	if fm.list.GetItemCount() > 0 {
		fm.updatePreview(fm.list.GetCurrentItem(), filteredEntries)
	}
}

// updatePreview updates the preview pane based on selected item
func (fm *FileManager) updatePreview(index int, filteredEntries []os.DirEntry) {
	if index >= len(filteredEntries) || index < 0 {
		fm.preview.SetText("Select a file or directory to preview")
		return
	}

	entry := filteredEntries[index]
	path := filepath.Join(fm.currentDir, entry.Name())
	info, err := os.Stat(path)
	if err != nil {
		fm.preview.SetText("[red]Error: " + err.Error() + "[-]")
		return
	}

	if entry.IsDir() {
		// Preview directory contents
		entries, err := os.ReadDir(path)
		if err != nil {
			fm.preview.SetText("[red]Error reading directory: " + err.Error() + "[-]")
			return
		}
		var previewLines []string
		previewLines = append(previewLines, "[blue]Directory: "+entry.Name()+"[-]")
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() {
				name += "/"
			}
			previewLines = append(previewLines, name)
		}
		fm.preview.SetText(strings.Join(previewLines, "\n"))
	} else {
		// Preview file
		mimeType := mime.TypeByExtension(filepath.Ext(entry.Name()))
		previewLines := []string{
			"[white]File: " + entry.Name() + "[-]",
			"Size: " + fmt.Sprintf("%d bytes", info.Size()),
			"Modified: " + info.ModTime().Format(time.RFC1123),
			"Permissions: " + info.Mode().String(),
			"MIME: " + map[string]string{"": "unknown", mimeType: mimeType}[mimeType],
		}

		if strings.HasPrefix(mimeType, "text/") {
			file, err := os.Open(path)
			if err != nil {
				previewLines = append(previewLines, "[red]Error reading file: "+err.Error()+"[-]")
			} else {
				defer file.Close()
				scanner := bufio.NewScanner(file)
				lineCount := 0
				previewLines = append(previewLines, "--- Content (first 20 lines) ---")
				for scanner.Scan() && lineCount < 20 {
					previewLines = append(previewLines, scanner.Text())
					lineCount++
				}
				if err := scanner.Err(); err != nil {
					previewLines = append(previewLines, "[red]Error reading content: "+err.Error()+"[-]")
				}
			}
		}
		fm.preview.SetText(strings.Join(previewLines, "\n"))
	}
}

// navigateTo changes to the specified directory
func (fm *FileManager) navigateTo(dir string) {
	newPath := filepath.Join(fm.currentDir, dir)
	if info, err := os.Stat(newPath); err == nil && info.IsDir() {
		fm.currentDir = newPath
		fm.searchQuery = ""
		fm.searchActive = false
		fm.listFlex.RemoveItem(fm.searchInput)
		fm.updateFileList()
		fm.app.SetFocus(fm.list)
	}
}

// navigateToParent navigates to the parent directory
func (fm *FileManager) navigateToParent() {
	fm.navigateTo("..")
}

// copyFile copies a file from src to dst
func (fm *FileManager) copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return err
	}

	// Copy file permissions
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	return os.Chmod(dst, info.Mode())
}

// copyDir recursively copies a directory from src to dst
func (fm *FileManager) copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := fm.copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := fm.copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// paste performs paste operation for the clipboard content
func (fm *FileManager) paste() {
	if fm.clipboardPath == "" {
		fm.preview.SetText("[red]Error: Nothing in clipboard[-]")
		return
	}

	dstPath := filepath.Join(fm.currentDir, filepath.Base(fm.clipboardPath))
	if _, err := os.Stat(dstPath); err == nil {
		fm.preview.SetText("[red]Error: Destination already exists[-]")
		return
	}

	info, err := os.Stat(fm.clipboardPath)
	if err != nil {
		fm.preview.SetText("[red]Error: " + err.Error() + "[-]")
		return
	}

	if info.IsDir() {
		err = fm.copyDir(fm.clipboardPath, dstPath)
	} else {
		err = fm.copyFile(fm.clipboardPath, dstPath)
	}
	if err != nil {
		fm.preview.SetText("[red]Error: " + err.Error() + "[-]")
		return
	}

	if fm.clipboardIsCut {
		if err := os.RemoveAll(fm.clipboardPath); err != nil {
			fm.preview.SetText("[red]Error removing source: " + err.Error() + "[-]")
			return
		}
		fm.clipboardPath = ""
		fm.clipboardIsCut = false
	}

	fm.preview.SetText("[green]Pasted: " + filepath.Base(dstPath) + "[-]")
	fm.updateFileList()
}

// rename renames the selected file or directory
func (fm *FileManager) rename(newName string) {
	selected := fm.list.GetCurrentItem()
	if selected >= fm.list.GetItemCount() {
		fm.preview.SetText("[red]Error: No item selected[-]")
		return
	}
	mainText, _ := fm.list.GetItemText(selected)
	cleanText := strings.TrimPrefix(strings.TrimSuffix(mainText, "[-]"), "[blue]")
	cleanText = strings.TrimPrefix(cleanText, "[white]")
	if len(cleanText) == 0 {
		fm.preview.SetText("[red]Error: Invalid selection[-]")
		return
	}
	oldPath := filepath.Join(fm.currentDir, strings.TrimSuffix(cleanText, "/"))
	newPath := filepath.Join(fm.currentDir, strings.TrimSpace(newName))
	if _, err := os.Stat(newPath); err == nil {
		fm.preview.SetText("[red]Error: Name already exists[-]")
		return
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		fm.preview.SetText("[red]Error: " + err.Error() + "[-]")
		return
	}
	fm.preview.SetText("[green]Renamed to: " + newName + "[-]")
	fm.updateFileList()
}

// delete deletes the selected file or directory
func (fm *FileManager) delete() {
	selected := fm.list.GetCurrentItem()
	if selected >= fm.list.GetItemCount() {
		fm.preview.SetText("[red]Error: No item selected[-]")
		return
	}
	mainText, _ := fm.list.GetItemText(selected)
	cleanText := strings.TrimPrefix(strings.TrimSuffix(mainText, "[-]"), "[blue]")
	cleanText = strings.TrimPrefix(cleanText, "[white]")
	if len(cleanText) == 0 {
		fm.preview.SetText("[red]Error: Invalid selection[-]")
		return
	}
	path := filepath.Join(fm.currentDir, strings.TrimSuffix(cleanText, "/"))
	if err := os.RemoveAll(path); err != nil {
		fm.preview.SetText("[red]Error: " + err.Error() + "[-]")
		return
	}
	fm.preview.SetText("[green]Deleted: " + filepath.Base(path) + "[-]")
	fm.updateFileList()
}

// setupKeyBindings configures key bindings for navigation
func (fm *FileManager) setupKeyBindings() {
	fm.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// If modal is active, only allow specific keys to pass to modal
		if fm.modalActive {
			switch event.Key() {
			case tcell.KeyEnter, tcell.KeyTab, tcell.KeyBacktab, tcell.KeyLeft, tcell.KeyRight, tcell.KeyEscape:
				return event
			default:
				return nil
			}
		}
		// If search or rename is active, only allow specific keys to pass to app
		if fm.searchActive || fm.renameActive {
			switch event.Key() {
			case tcell.KeyEnter, tcell.KeyEscape:
				// Handle in searchInput or renameInput's DoneFunc
				return event
			default:
				// Let search or rename input handle other keys
				return event
			}
		}

		switch event.Key() {
		case tcell.KeyEnter, tcell.KeyRight:
			selected := fm.list.GetCurrentItem()
			mainText, _ := fm.list.GetItemText(selected)
			// Remove color tags for navigation
			cleanText := strings.TrimPrefix(strings.TrimSuffix(mainText, "[-]"), "[blue]")
			cleanText = strings.TrimPrefix(cleanText, "[white]")
			if len(cleanText) > 0 && cleanText[len(cleanText)-1] == '/' {
				fm.navigateTo(cleanText[:len(cleanText)-1])
			}
			return nil		
		case tcell.KeyLeft:
			fm.navigateToParent()
			return nil
		case tcell.KeyCtrlC:
			selected := fm.list.GetCurrentItem()
			if selected < fm.list.GetItemCount() {
				mainText, _ := fm.list.GetItemText(selected)
				cleanText := strings.TrimPrefix(strings.TrimSuffix(mainText, "[-]"), "[blue]")
				cleanText = strings.TrimPrefix(cleanText, "[white]")
				if len(cleanText) > 0 {
					fm.clipboardPath = filepath.Join(fm.currentDir, strings.TrimSuffix(cleanText, "/"))
					fm.clipboardIsCut = false
					fm.preview.SetText("[green]Copied: " + filepath.Base(fm.clipboardPath) + "[-]")
				}
			}
			return nil
		case tcell.KeyCtrlX:
			selected := fm.list.GetCurrentItem()
			if selected < fm.list.GetItemCount() {
				mainText, _ := fm.list.GetItemText(selected)
				cleanText := strings.TrimPrefix(strings.TrimSuffix(mainText, "[-]"), "[blue]")
				cleanText = strings.TrimPrefix(cleanText, "[white]")
				if len(cleanText) > 0 {
					fm.clipboardPath = filepath.Join(fm.currentDir, strings.TrimSuffix(cleanText, "/"))
					fm.clipboardIsCut = true
					fm.preview.SetText("[green]Cut: " + filepath.Base(fm.clipboardPath) + "[-]")
				}
			}
			return nil
		case tcell.KeyCtrlV:
			fm.paste()
			return nil
		case tcell.KeyCtrlR:
			selected := fm.list.GetCurrentItem()
			if selected < fm.list.GetItemCount() {
				fm.renameActive = true
				fm.listFlex.AddItem(fm.renameInput, 1, 1, false)
				fm.app.SetFocus(fm.renameInput)
			}
			return nil
		case tcell.KeyCtrlD:
			selected := fm.list.GetCurrentItem()
			if selected < fm.list.GetItemCount() {
				mainText, _ := fm.list.GetItemText(selected)
				cleanText := strings.TrimPrefix(strings.TrimSuffix(mainText, "[-]"), "[blue]")
				cleanText = strings.TrimPrefix(cleanText, "[white]")
				fm.confirmModal.SetText("Confirm delete: " + cleanText + "?")
				fm.modalActive = true
				fm.app.SetRoot(tview.NewFlex().
					AddItem(fm.listFlex, 0, 1, false).
					AddItem(fm.preview, 0, 1, false).
					AddItem(fm.confirmModal, 0, 0, true), true)
				fm.app.SetFocus(fm.confirmModal)
			}
			return nil
		case tcell.KeyRune:
			switch event.Rune() {
			case 's', 'S': // Toggle hidden files with S
				fm.showHidden = !fm.showHidden
				fm.updateFileList()
				return nil
			case 'f', 'F': // Start search with F
				fm.searchActive = true
				fm.listFlex.AddItem(fm.searchInput, 1, 1, false)
				fm.app.SetFocus(fm.searchInput)
				return nil
			}
		case tcell.KeyEscape:
			fm.app.Stop()
			return nil
		}
		return event
	})

	// Search input key bindings
	fm.searchInput.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			fm.searchQuery = strings.TrimSpace(fm.searchInput.GetText())
			fm.searchActive = false
			fm.listFlex.RemoveItem(fm.searchInput)
			fm.updateFileList()
			fm.app.SetFocus(fm.list)
		} else if key == tcell.KeyEscape {
			fm.searchQuery = ""
			fm.searchActive = false
			fm.listFlex.RemoveItem(fm.searchInput)
			fm.searchInput.SetText("")
			fm.updateFileList()
			fm.app.SetFocus(fm.list)
		}
	}).SetChangedFunc(func(text string) {
		fm.searchQuery = strings.TrimSpace(text)
		fm.updateFileList()
	})

	// Rename input key bindings
	fm.renameInput.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			newName := strings.TrimSpace(fm.renameInput.GetText())
			if newName != "" {
				fm.rename(newName)
			}
			fm.renameActive = false
			fm.listFlex.RemoveItem(fm.renameInput)
			fm.renameInput.SetText("")
			fm.app.SetFocus(fm.list)
		} else if key == tcell.KeyEscape {
			fm.renameActive = false
			fm.listFlex.RemoveItem(fm.renameInput)
			fm.renameInput.SetText("")
			fm.app.SetFocus(fm.list)
		}
	})

	// Configure confirm modal for delete
	fm.confirmModal.SetText("Confirm delete?").
		AddButtons([]string{"Yes", "No"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			fm.modalActive = false
			fm.app.SetRoot(tview.NewFlex().
				AddItem(fm.listFlex, 0, 1, true).
				AddItem(fm.preview, 0, 1, false), true)
			if buttonLabel == "Yes" {
				fm.delete()
			} else {
				fm.preview.SetText("[yellow]Delete canceled[-]")
			}
		})
}

func main() {
	fm := NewFileManager()
	fm.setupUI()
}