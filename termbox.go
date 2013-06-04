// +build !windows

package termbox

import (
    "os"
    "strings"
    "syscall"
    "unicode/utf8"
    "unsafe"
)

// private API

const (
	t_enter_ca = iota
	t_exit_ca
	t_show_cursor
	t_hide_cursor
	t_clear_screen
	t_sgr0
	t_underline
	t_bold
	t_blink
	t_reverse
	t_enter_keypad
	t_exit_keypad
	t_max_funcs
)

const (
	coord_invalid = -2
	attr_invalid  = Attribute(0xFFFF)
)

type input_event struct {
	data []byte
	err  error
}

var (
	// term specific sequences
	keys  []string
	funcs []string

	// termbox inner state
	orig_tios    syscall_Termios
	back_buffer  cellbuf
	front_buffer cellbuf
	termw        int
	termh        int
	input_mode   = InputEsc
	out          *os.File
	in           int
	lastfg       = attr_invalid
	lastbg       = attr_invalid
	lastx        = coord_invalid
	lasty        = coord_invalid
	cursor_x     = cursor_hidden
	cursor_y     = cursor_hidden
	foreground   = ColorDefault
	background   = ColorDefault
	inbuf        = make([]byte, 0, 64)
	sigwinch     = make(chan os.Signal, 1)
	sigio        = make(chan os.Signal, 1)
	quit         = make(chan int)
	input_comm   = make(chan input_event)

    // Buffer loaded with escape sequences that will be dumped to the terminal
    // when flush() is called.
    //
    // We're hardcoding the size of the main storage which could get us into a
    // panic if we come up with an application where we're dumping too much
    // to the screen at a time.
    esmain = make([]byte, 0, 200000) // Main storage
    esub   = esmain[0:0]             // Sub-buffer within main storage
)

func write_cursor(x, y int) {
    y += 1
    x += 1

    esub   = esmain[len(esmain):len(esmain)+10]
    esmain = esmain[           :len(esmain)+len(esub)]

    esub[0] = '\033'
    esub[1] = '['
    switch {
    case y <   10:
        esub[2] = '0'
        esub[3] = '0'
        esub[4] = byte(y)+'0'
    case y <  100:
        esub[2] = '0'
        esub[3] = (byte(y)/10)+'0'
        esub[4] = (byte(y)%10)+'0'
    case y < 1000:
        hundreds := y/100
        y -= hundreds*100
        esub[2] = byte(hundreds)+'0'
        esub[3] = (byte(y)/10)+'0'
        esub[4] = (byte(y)%10)+'0'
    }
    esub[5] = ';'
    switch {
    case x <   10:
        esub[6] = '0'
        esub[7] = '0'
        esub[8] = byte(x)+'0'
    case x <  100:
        esub[6] = '0'
        esub[7] = (byte(x)/10)+'0'
        esub[8] = (byte(x)%10)+'0'
    case x < 1000:
        hundreds := x/100
        x -= hundreds*100
        esub[6] = byte(hundreds)+'0'
        esub[7] = (byte(x)/10)+'0'
        esub[8] = (byte(x)%10)+'0'
    }
    esub[9] = 'H'
}

func write_sgr_fg(a Attribute) {
    esub   = esmain[len(esmain):len(esmain)+10]
    esmain = esmain[           :len(esmain)+len(esub)]

    esub[0] = '\033'
    esub[1] = '['
    esub[2] = '3'
    esub[3] = '8'
    esub[4] = ';'
    esub[5] = '5'
    esub[6] = ';'
    switch a {
    case ColorBlack:        esub[7] = '0' ; esub[8] = '0'
    case ColorRed:          esub[7] = '0' ; esub[8] = '1'
    case ColorGreen:        esub[7] = '0' ; esub[8] = '2'
    case ColorYellow:       esub[7] = '0' ; esub[8] = '3'
    case ColorBlue:         esub[7] = '0' ; esub[8] = '4'
    case ColorMagenta:      esub[7] = '0' ; esub[8] = '5'
    case ColorCyan:         esub[7] = '0' ; esub[8] = '6'
    case ColorLightGray:    esub[7] = '0' ; esub[8] = '7'
    case ColorDarkGray:     esub[7] = '0' ; esub[8] = '8'
    case ColorLightRed:     esub[7] = '0' ; esub[8] = '9'
    case ColorLightGreen:   esub[7] = '1' ; esub[8] = '0'
    case ColorLightYellow:  esub[7] = '1' ; esub[8] = '1'
    case ColorLightBlue:    esub[7] = '1' ; esub[8] = '2'
    case ColorLightMagenta: esub[7] = '1' ; esub[8] = '3'
    case ColorLightCyan:    esub[7] = '1' ; esub[8] = '4'
    case ColorWhite:        esub[7] = '1' ; esub[8] = '5'
    }
    esub[9] = 'm'
}

func write_sgr_bg(a Attribute) {
    esub   = esmain[len(esmain):len(esmain)+10]
    esmain = esmain[           :len(esmain)+len(esub)]

    esub[0] = '\033'
    esub[1] = '['
    esub[2] = '4'
    esub[3] = '8'
    esub[4] = ';'
    esub[5] = '5'
    esub[6] = ';'
    switch a {
    case ColorBlack:        esub[7] = '0' ; esub[8] = '0'
    case ColorRed:          esub[7] = '0' ; esub[8] = '1'
    case ColorGreen:        esub[7] = '0' ; esub[8] = '2'
    case ColorYellow:       esub[7] = '0' ; esub[8] = '3'
    case ColorBlue:         esub[7] = '0' ; esub[8] = '4'
    case ColorMagenta:      esub[7] = '0' ; esub[8] = '5'
    case ColorCyan:         esub[7] = '0' ; esub[8] = '6'
    case ColorLightGray:    esub[7] = '0' ; esub[8] = '7'
    case ColorDarkGray:     esub[7] = '0' ; esub[8] = '8'
    case ColorLightRed:     esub[7] = '0' ; esub[8] = '9'
    case ColorLightGreen:   esub[7] = '1' ; esub[8] = '0'
    case ColorLightYellow:  esub[7] = '1' ; esub[8] = '1'
    case ColorLightBlue:    esub[7] = '1' ; esub[8] = '2'
    case ColorLightMagenta: esub[7] = '1' ; esub[8] = '3'
    case ColorLightCyan:    esub[7] = '1' ; esub[8] = '4'
    case ColorWhite:        esub[7] = '1' ; esub[8] = '5'
    }
    esub[9] = 'm'
}

type winsize struct {
	rows    uint16
	cols    uint16
	xpixels uint16
	ypixels uint16
}

func get_term_size(fd uintptr) (int, int) {
	var sz winsize
	_, _, _ = syscall.Syscall(syscall.SYS_IOCTL,
		fd, uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&sz)))
	return int(sz.cols), int(sz.rows)
}

func send_attr(fg, bg Attribute) {
	if fg != lastfg || bg != lastbg {
        esub   = esmain[len(esmain):len(esmain)+len(funcs[t_sgr0])]
        esmain = esmain[           :len(esmain)+len(esub)]
        copy(esub, funcs[t_sgr0])

		fgcol := fg & 0x00FF
		bgcol := bg & 0x00FF
		if fgcol != ColorDefault {
            write_sgr_fg(fgcol)
        }
		if bgcol != ColorDefault {
			write_sgr_bg(bgcol)
		}

		if fg&AttrBold != 0 {
            esub   = esmain[len(esmain):len(esmain)+len(funcs[t_bold])]
            esmain = esmain[           :len(esmain)+len(esub)]
            copy(esub, funcs[t_bold])
		}
		if fg&AttrBlink|bg&AttrBlink != 0 {
            esub   = esmain[len(esmain):len(esmain)+len(funcs[t_blink])]
            esmain = esmain[           :len(esmain)+len(esub)]
            copy(esub, funcs[t_blink])
		}
		if fg&AttrUnderline != 0 {
            esub   = esmain[len(esmain):len(esmain)+len(funcs[t_underline])]
            esmain = esmain[           :len(esmain)+len(esub)]
            copy(esub, funcs[t_underline])
		}
		if fg&AttrReverse|bg&AttrReverse != 0 {
            esub   = esmain[len(esmain):len(esmain)+len(funcs[t_reverse])]
            esmain = esmain[           :len(esmain)+len(esub)]
            copy(esub, funcs[t_reverse])
		}

		lastfg, lastbg = fg, bg
	}
}

var (
    send_char_array [8]byte
    send_char_slice_1 = send_char_array[:1]
    send_char_slice_2 = send_char_array[:2]
    send_char_slice_3 = send_char_array[:3]
    send_char_slice_4 = send_char_array[:4]
    send_char_slice_5 = send_char_array[:5]
    send_char_slice_6 = send_char_array[:6]
    send_char_slice_7 = send_char_array[:7]
    send_char_slice_8 = send_char_array[:8]
)

func send_char(x, y int, ch rune) {
	n := utf8.EncodeRune(send_char_slice_8, ch)
	if x-1 != lastx || y != lasty {
		write_cursor(x, y)
	}
	lastx, lasty = x, y

    var es *[]byte // escape sequence
    switch n {
    case 1: es = &send_char_slice_1
    case 2: es = &send_char_slice_2
    case 3: es = &send_char_slice_3
    case 4: es = &send_char_slice_4
    case 5: es = &send_char_slice_5
    case 6: es = &send_char_slice_6
    case 7: es = &send_char_slice_7
    case 8: es = &send_char_slice_8
    }
    esub   = esmain[len(esmain):len(esmain)+len(*es)]
    esmain = esmain[           :len(esmain)+len(esub)]
    copy(esub, *es)
}

func flush() error {
    _, err := out.Write(esmain)
    esub   = esmain[0:0]
    esmain = esmain[0:0]
	if err != nil {
		return err
	}
	return nil
}

func send_clear() error {
	send_attr(foreground, background)
    esub   = esmain[len(esmain):len(esmain)+len(funcs[t_clear_screen])]
    esmain = esmain[           :len(esmain)+len(esub)]
    copy(esub, funcs[t_clear_screen])
	if !is_cursor_hidden(cursor_x, cursor_y) {
		write_cursor(cursor_x, cursor_y)
	}

	// we need to invalidate cursor position too and these two vars are
	// used only for simple cursor positioning optimization, cursor
	// actually may be in the correct place, but we simply discard
	// optimization once and it gives us simple solution for the case when
	// cursor moved
	lastx = coord_invalid
	lasty = coord_invalid

	return flush()
}

func update_size_maybe() error {
	w, h := get_term_size(out.Fd())
	if w != termw || h != termh {
		termw, termh = w, h
		back_buffer.resize(termw, termh)
		front_buffer.resize(termw, termh)
		front_buffer.clear()
		return send_clear()
	}
	return nil
}

func tcsetattr(fd uintptr, termios *syscall_Termios) error {
	r, _, e := syscall.Syscall(syscall.SYS_IOCTL,
		fd, uintptr(syscall_TCSETS), uintptr(unsafe.Pointer(termios)))
	if r != 0 {
		return os.NewSyscallError("SYS_IOCTL", e)
	}
	return nil
}

func tcgetattr(fd uintptr, termios *syscall_Termios) error {
	r, _, e := syscall.Syscall(syscall.SYS_IOCTL,
		fd, uintptr(syscall_TCGETS), uintptr(unsafe.Pointer(termios)))
	if r != 0 {
		return os.NewSyscallError("SYS_IOCTL", e)
	}
	return nil
}

func parse_escape_sequence(event *Event, buf []byte) int {
	bufstr := string(buf)
	for i, key := range keys {
		if strings.HasPrefix(bufstr, key) {
			event.Ch = 0
			event.Key = Key(0xFFFF - i)
			return len(key)
		}
	}
	return 0
}

func extract_event(event *Event) bool {
	if len(inbuf) == 0 {
		return false
	}

	if inbuf[0] == '\033' {
		// possible escape sequence
		n := parse_escape_sequence(event, inbuf)
		if n != 0 {
			copy(inbuf, inbuf[n:])
			inbuf = inbuf[:len(inbuf)-n]
			return true
		}

		// it's not escape sequence, then it's Alt or Esc, check input_mode
		switch input_mode {
		case InputEsc:
			// if we're in escape mode, fill Esc event, pop buffer, return success
			event.Ch = 0
			event.Key = KeyEsc
			event.Mod = 0
			copy(inbuf, inbuf[1:])
			inbuf = inbuf[:len(inbuf)-1]
			return true
		case InputAlt:
			// if we're in alt mode, set Alt modifier to event and redo parsing
			event.Mod = ModAlt
			copy(inbuf, inbuf[1:])
			inbuf = inbuf[:len(inbuf)-1]
			return extract_event(event)
		default:
			panic("unreachable")
		}
	}

	// if we're here, this is not an escape sequence and not an alt sequence
	// so, it's a FUNCTIONAL KEY or a UNICODE character

	// first of all check if it's a functional key
	if Key(inbuf[0]) <= KeySpace || Key(inbuf[0]) == KeyBackspace2 {
		// fill event, pop buffer, return success
		event.Ch = 0
		event.Key = Key(inbuf[0])
		copy(inbuf, inbuf[1:])
		inbuf = inbuf[:len(inbuf)-1]
		return true
	}

	// the only possible option is utf8 rune
	if r, n := utf8.DecodeRune(inbuf); r != utf8.RuneError {
		event.Ch = r
		event.Key = 0
		copy(inbuf, inbuf[n:])
		inbuf = inbuf[:len(inbuf)-n]
		return true
	}

	return false
}

func fcntl(fd int, cmd int, arg int) (val int, err error) {
	r, _, e := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), uintptr(cmd),
		uintptr(arg))
	val = int(r)
	if e != 0 {
		err = e
	}
	return
}
