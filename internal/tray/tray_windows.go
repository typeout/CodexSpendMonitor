package tray

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

const (
	trayMessage  = win.WM_USER + 1
	ninSelect    = win.WM_USER
	ninKeySelect = win.WM_USER + 1
	menuOpen     = 1001
	menuQuit     = 1002
	trayUID      = 1
)

type Options struct {
	Tooltip      string
	IconPath     string
	TodaySummary func() string
	OnOpen       func()
	OnQuit       func()
}

type runner struct {
	options Options
	hwnd    win.HWND
	icon    win.HICON
	nid     win.NOTIFYICONDATA
}

var active *runner

func Run(options Options) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	instance := win.GetModuleHandle(nil)
	if instance == 0 {
		return fmt.Errorf("getting module handle failed")
	}

	className, err := windows.UTF16PtrFromString("CodexSpendMonitorTrayWindow")
	if err != nil {
		return err
	}

	active = &runner{options: options}
	wc := win.WNDCLASSEX{
		CbSize:        uint32(unsafe.Sizeof(win.WNDCLASSEX{})),
		LpfnWndProc:   syscall.NewCallback(windowProc),
		HInstance:     instance,
		LpszClassName: className,
	}
	if atom := win.RegisterClassEx(&wc); atom == 0 {
		return fmt.Errorf("registering tray window class failed")
	}

	hwnd := win.CreateWindowEx(
		0,
		className,
		className,
		0,
		0, 0, 0, 0,
		0,
		0,
		instance,
		nil,
	)
	if hwnd == 0 {
		return fmt.Errorf("creating tray window failed")
	}
	active.hwnd = hwnd

	active.icon = loadTrayIcon(options.IconPath)
	active.nid = win.NOTIFYICONDATA{
		CbSize:           uint32(unsafe.Sizeof(win.NOTIFYICONDATA{})),
		HWnd:             hwnd,
		UID:              trayUID,
		UFlags:           win.NIF_MESSAGE | win.NIF_ICON | win.NIF_TIP,
		UCallbackMessage: trayMessage,
		HIcon:            active.icon,
	}
	copyUTF16(active.nid.SzTip[:], options.Tooltip)
	if ok := win.Shell_NotifyIcon(win.NIM_ADD, &active.nid); !ok {
		win.DestroyWindow(hwnd)
		return fmt.Errorf("adding tray icon failed")
	}
	defer win.Shell_NotifyIcon(win.NIM_DELETE, &active.nid)
	defer win.DestroyWindow(hwnd)

	var msg win.MSG
	for win.GetMessage(&msg, 0, 0, 0) > 0 {
		win.TranslateMessage(&msg)
		win.DispatchMessage(&msg)
	}
	return nil
}

func Quit() {
	if active != nil && active.hwnd != 0 {
		win.DestroyWindow(active.hwnd)
		return
	}
	win.PostQuitMessage(0)
}

func windowProc(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case trayMessage:
		switch uint32(lParam) {
		case win.WM_LBUTTONUP, win.WM_LBUTTONDBLCLK, ninSelect, ninKeySelect:
			openDashboard()
			return 0
		case win.WM_RBUTTONUP:
			showMenu(hwnd)
			return 0
		}
	case win.WM_COMMAND:
		runCommand(hwnd, uint16(wParam&0xffff))
		return 0
	case win.WM_DESTROY:
		win.PostQuitMessage(0)
		return 0
	}
	return win.DefWindowProc(hwnd, msg, wParam, lParam)
}

func showMenu(hwnd win.HWND) {
	menu := win.CreatePopupMenu()
	if menu == 0 {
		return
	}
	defer win.DestroyMenu(menu)

	todayText := "Today: unavailable"
	if active != nil && active.options.TodaySummary != nil {
		todayText = active.options.TodaySummary()
	}
	appendMenuItem(menu, 0, todayText, win.MFT_STRING, win.MFS_DISABLED)
	insertSeparator(menu, 1)
	appendMenu(menu, menuOpen, "Open Dashboard")
	insertSeparator(menu, 3)
	appendMenu(menu, menuQuit, "Quit")

	var point win.POINT
	if !win.GetCursorPos(&point) {
		return
	}
	win.SetForegroundWindow(hwnd)
	command := win.TrackPopupMenu(
		menu,
		win.TPM_RIGHTBUTTON|win.TPM_RETURNCMD|win.TPM_NONOTIFY,
		point.X,
		point.Y,
		0,
		hwnd,
		nil,
	)
	if command != 0 {
		runCommand(hwnd, uint16(command))
	}
	win.DefWindowProc(hwnd, win.WM_NULL, 0, 0)
}

func runCommand(hwnd win.HWND, command uint16) {
	switch command {
	case menuOpen:
		openDashboard()
	case menuQuit:
		if active != nil && active.options.OnQuit != nil {
			active.options.OnQuit()
		}
		win.DestroyWindow(hwnd)
	}
}

func openDashboard() {
	if active != nil && active.options.OnOpen != nil {
		active.options.OnOpen()
	}
}

func appendMenu(menu win.HMENU, id uintptr, text string) {
	appendMenuItem(menu, id, text, win.MFT_STRING, win.MFS_ENABLED)
}

func appendMenuItem(menu win.HMENU, id uintptr, text string, itemType uint32, itemState uint32) {
	ptr, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return
	}
	item := win.MENUITEMINFO{
		CbSize: uint32(unsafe.Sizeof(win.MENUITEMINFO{})),
		FMask:  win.MIIM_STRING | win.MIIM_FTYPE | win.MIIM_STATE,
		FType:  itemType,
		FState: itemState,
	}
	if id != 0 {
		item.FMask |= win.MIIM_ID
		item.WID = uint32(id)
	}
	item.DwTypeData = ptr
	item.Cch = uint32(len(windows.StringToUTF16(text)) - 1)
	win.InsertMenuItem(menu, uint32(win.GetMenuItemCount(menu)), true, &item)
}

func insertSeparator(menu win.HMENU, position uint32) {
	item := win.MENUITEMINFO{
		CbSize: uint32(unsafe.Sizeof(win.MENUITEMINFO{})),
		FMask:  win.MIIM_FTYPE,
		FType:  win.MFT_SEPARATOR,
	}
	win.InsertMenuItem(menu, position, true, &item)
}

func copyUTF16(dst []uint16, value string) {
	encoded := windows.StringToUTF16(value)
	if len(encoded) > len(dst) {
		encoded = encoded[:len(dst)]
		encoded[len(encoded)-1] = 0
	}
	copy(dst, encoded)
}

func loadTrayIcon(path string) win.HICON {
	if path != "" {
		if icon, err := iconFromPNG(path, 32); err == nil && icon != 0 {
			return icon
		}
	}
	return win.LoadIcon(0, win.MAKEINTRESOURCE(win.IDI_APPLICATION))
}

func iconFromPNG(path string, size int) (win.HICON, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	src, err := png.Decode(file)
	if err != nil {
		return 0, err
	}
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	srcBounds := src.Bounds()
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			srcX := srcBounds.Min.X + x*srcBounds.Dx()/size
			srcY := srcBounds.Min.Y + y*srcBounds.Dy()/size
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}

	colorBits := make([]byte, size*size*4)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			r, g, b, a := dst.At(x, y).RGBA()
			offset := ((size-1-y)*size + x) * 4
			colorBits[offset+0] = byte(b >> 8)
			colorBits[offset+1] = byte(g >> 8)
			colorBits[offset+2] = byte(r >> 8)
			colorBits[offset+3] = byte(a >> 8)
		}
	}
	maskBits := make([]byte, ((size+15)/16)*2*size)

	colorBitmap := win.CreateBitmap(int32(size), int32(size), 1, 32, unsafe.Pointer(&colorBits[0]))
	if colorBitmap == 0 {
		return 0, fmt.Errorf("creating color bitmap failed")
	}
	defer win.DeleteObject(win.HGDIOBJ(colorBitmap))

	maskBitmap := win.CreateBitmap(int32(size), int32(size), 1, 1, unsafe.Pointer(&maskBits[0]))
	if maskBitmap == 0 {
		return 0, fmt.Errorf("creating mask bitmap failed")
	}
	defer win.DeleteObject(win.HGDIOBJ(maskBitmap))

	info := win.ICONINFO{
		FIcon:    1,
		HbmColor: colorBitmap,
		HbmMask:  maskBitmap,
	}
	icon := win.CreateIconIndirect(&info)
	if icon == 0 {
		return 0, fmt.Errorf("creating icon failed")
	}
	return icon, nil
}
