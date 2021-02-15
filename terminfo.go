// package terminfo is a pure-go library for reading compiled term
// files as described in term(5).
//
// Only directory-tree descriptions are supported right now (patches welcome!).
package terminfo // import "gopkg.in/terminfo.v0"

//go:generate find . -name termh*.go -delete
//go:generate ./gen-termh.py
//go:generate stringer -type BooleanIndex,NumberIndex,StringIndex -output termh_string.go

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image/color"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/termios.v0"
)

// ComipledInLocations is the list of “compiled-in” locations that
// ncurses would search. The default value matches what Ubuntu does;
// adjust as necessary before calling anything else.
var CompiledInLocations = []string{"/etc/terminfo", "/lib/terminfo", "/usr/share/terminfo"}

func home() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	if u, err := user.Current(); err == nil {
		return u.HomeDir
	}
	// aarghbglbgl
	return "/"
}

type paramStack struct {
	stack []interface{}
}

func (p *paramStack) push(arg interface{}) {
	switch arg.(type) {
	case string, int:
		// ok
	default:
		panic(fmt.Errorf("unexpected type %T in stack.push", arg))
	}
	p.stack = append(p.stack, arg)
}

func (p *paramStack) pushBool(arg bool) {
	if arg {
		p.push(1)
	} else {
		p.push(0)
	}
}

func (p *paramStack) pop() interface{} {
	x := p.stack[len(p.stack)-1]
	p.stack = p.stack[:len(p.stack)-1]
	return x
}

func (p *paramStack) popInt() int {
	return p.pop().(int)
}

func (p *paramStack) popBool() bool {
	return p.popInt() != 0
}

func (p *paramStack) popString() string {
	return p.pop().(string)
}

func UnescapeString(tpl string, args ...interface{}) ([]byte, error) {
	return Unescape([]byte(tpl), args...)
}
func Unescape(tpl []byte, args ...interface{}) ([]byte, error) {
	var buf []byte
	var stack paramStack
	isParam := false
	for i := 0; i < len(tpl); i++ {
		if !isParam {
			if tpl[i] == '%' {
				isParam = true
			} else {
				buf = append(buf, tpl[i])
			}
			continue
		}
		isParam = false
		switch tpl[i] {
		case '%':
			buf = append(buf, '%')
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', ':', '#', ' ', 'd', 'o', 'x', 'X', 's', 'c':
			done := false
			var j int
		printfLikeInner:
			for j = i; j < len(tpl); j++ {
				switch tpl[j] {
				case 'd', 'o', 'x', 'X', 's', 'c':
					done = true
					break printfLikeInner
				}
			}
			if !done {
				return nil, ErrTruncatedParametrizedString
			}
			f := fmt.Sprintf("%%%s", tpl[i:j+1])
			gen := fmt.Sprintf(f, stack.pop())
			i = j
			buf = append(buf, []byte(gen)...)
		case 'p':
			if len(tpl) <= i+1 {
				return nil, ErrTruncatedParametrizedString
			}
			i++
			n, err := strconv.Atoi(string(tpl[i : i+1]))
			if err != nil {
				return nil, err
			}
			if n > len(args) {
				return nil, ErrMissingArgs
			}
			stack.push(args[n-1])
		case 'P', 'g':
			return nil, ErrNotImplemented
		case '\'':
			if len(tpl) <= i+2 {
				return nil, ErrTruncatedParametrizedString
			}
			i++
			stack.push(int(tpl[i]))
			i++
			if tpl[i] != '\'' {
				return nil, ErrBadParametrizedString
			}
		case '{':
			n := 0
		literalNumberInner:
			for j := i + 1; j < len(tpl); j++ {
				switch tpl[j] {
				case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
					n = n*10 + int(tpl[j]-'0')
				case '}':
					i = j
					break literalNumberInner
				default:
					return nil, ErrBadParametrizedString
				}
			}
			stack.push(n)
		case 'l':
			stack.push(len(stack.popString()))
		case '+':
			stack.push(stack.popInt() + stack.popInt())
		case '-':
			stack.push(-stack.popInt() + stack.popInt())
		case '*':
			stack.push(stack.popInt() * stack.popInt())
		case '/':
			n1 := stack.popInt()
			n2 := stack.popInt()
			stack.push(n2 / n1)
		case 'm':
			n1 := stack.popInt()
			n2 := stack.popInt()
			stack.push(n2 % n1)
		case '&':
			stack.push(stack.popInt() & stack.popInt())
		case '|':
			stack.push(stack.popInt() | stack.popInt())
		case '^':
			stack.push(stack.popInt() ^ stack.popInt())
		case '=':
			stack.pushBool(stack.popInt() == stack.popInt())
		case '<':
			stack.pushBool(stack.popInt() > stack.popInt())
		case '>':
			stack.pushBool(stack.popInt() < stack.popInt())
		case 'A':
			stack.pushBool(stack.popBool() && stack.popBool())
		case 'O':
			stack.pushBool(stack.popBool() || stack.popBool())
		case '!':
			stack.pushBool(!stack.popBool())
		case '~':
			// TODO: find an example of this to check word size & etc
			stack.push(^stack.popInt())
		case 'i':
			switch len(args) {
			default:
				args[1] = args[1].(int) + 1
				fallthrough
			case 1:
				args[0] = args[0].(int) + 1
			case 0:
				// do naught
			}
		case '?', ';':
			// return nil, ErrNotImplemented
		case 't':
			if stack.popBool() {
				// test case was true; continue as usual
				continue
			}
			// test case was false; look for %e or %;
			fallthrough
		case 'e':
			elseOk := tpl[i] == 't'
			ok := false
			for i = i + 1; i < len(tpl)-1; i++ {
				if tpl[i] == '%' && (tpl[i+1] == ';' || (elseOk && tpl[i+1] == 'e')) {
					ok = true
					i++
					break
				}
			}
			if !ok {
				return nil, ErrBadParametrizedString
			}
		}
	}
	return buf, nil
}

type TermInfo struct {
	Names      []string
	Booleans   []bool
	Numbers    []int16
	BigNumbers []int32
	Strings    map[StringIndex][]byte
	tty        *os.File
}

var findPadIndexes = regexp.MustCompile(`\$<(\d+)(\*)?(/)?>`).FindAllSubmatchIndex

func (ti *TermInfo) pad(n int, mandatory bool) {
	// NOTE this seems to be right, but also seems to produce very
	// different results from what `tput` does.
	var padSeq []byte
	var numPad int
	ospeed := -1
	if !mandatory {
		if ti.Booleans[XonXoff] {
			return
		}
		var minBaudRate int
		if len(ti.BigNumbers) > 0 {
			minBaudRate = int(ti.BigNumbers[PaddingBaudRate])
		} else {
			minBaudRate = int(ti.Numbers[PaddingBaudRate])
		}
		if minBaudRate < 0 {
			return
		}
		if tio, err := termios.GetAttr(ti.tty.Fd()); err == nil {
			_, ospeed = tio.GetSpeed()
		}
		if ospeed < minBaudRate {
			return
		}
	}

	if ti.Booleans[NoPadChar] || ti.Booleans[XonXoff] {
		goto sleep
	}
	if ospeed < 0 {
		if tio, err := termios.GetAttr(ti.tty.Fd()); err == nil {
			_, ospeed = tio.GetSpeed()
		}
		if ospeed < 0 {
			goto sleep
		}
	}
	padSeq = []byte(ti.Strings[PadChar])
	if len(padSeq) == 0 {
		padSeq = []byte{0}
	}

	numPad = n * ospeed / 9000 / len(padSeq)
	for i := 0; i < numPad; i++ {
		ti.tty.Write(padSeq)
	}

	return
sleep:
	time.Sleep(time.Duration(n) * time.Millisecond)
}

func (ti *TermInfo) MustUnescape(idx StringIndex, args ...interface{}) string {
	buf, err := Unescape(ti.Strings[idx], args...)
	if err != nil {
		panic(err)
	}
	return string(buf)
}

var (
	Black     = color.RGBA{0, 0, 0, 255}
	Red       = color.RGBA{205, 0, 0, 255}
	Green     = color.RGBA{0, 205, 0, 255}
	Orange    = color.RGBA{205, 205, 0, 255}
	Blue      = color.RGBA{0, 0, 238, 255}
	Magenta   = color.RGBA{205, 0, 205, 255}
	Cyan      = color.RGBA{0, 205, 205, 255}
	LightGrey = color.RGBA{229, 229, 229, 255}

	DarkGrey     = color.RGBA{127, 127, 127, 255}
	LightRed     = color.RGBA{255, 0, 0, 255}
	LightGreen   = color.RGBA{0, 255, 0, 255}
	Yellow       = color.RGBA{255, 255, 0, 255}
	LightBlue    = color.RGBA{92, 92, 255, 255}
	LightMagenta = color.RGBA{255, 0, 255, 255}
	LightCyan    = color.RGBA{0, 255, 255, 255}
	White        = color.RGBA{255, 255, 255, 255}
)

var xterm = color.Palette{
	// dark colors:
	Black,
	Red,
	Green,
	Orange,
	Blue,
	Magenta,
	Cyan,
	LightGrey,
	// light colors:
	DarkGrey,
	LightRed,
	LightGreen,
	Yellow,
	LightBlue,
	LightMagenta,
	LightCyan,
	White,
}

func (ti *TermInfo) Color(fg, bg color.Color) string {
	if int(MaxColors) >= len(ti.Numbers) && int(MaxColors) > len(ti.BigNumbers) {
		return ""
	}
	var cols int32
	if len(ti.BigNumbers) > 0 {
		cols = ti.BigNumbers[MaxColors]
	} else {
		cols = int32(ti.Numbers[MaxColors])
	}
	if cols <= 0 {
		return ""
	}
	if cols < 88 {
		fgi := xterm.Index(fg)
		bgi := xterm.Index(bg)
		return ti.MustUnescape(SetAForeground, fgi) + ti.MustUnescape(SetABackground, bgi)
	}
	// assume it supports ISO-8613-3
	fR, fG, fB, fA := fg.RGBA()
	bR, bG, bB, bA := bg.RGBA()
	fQ := fA / 255
	bQ := bA / 255
	return fmt.Sprintf("\033[38;2;%d;%d;%d;48;2;%d;%d;%dm",
		fR/fQ, fG/fQ, fB/fQ,
		bR/bQ, bG/bQ, bB/bQ)
}

func (ti *TermInfo) Puts(idx StringIndex, affcnt int, args ...interface{}) error {
	buf, err := Unescape(ti.Strings[idx], args...)
	if err != nil {
		return err
	}
	padIndexes := findPadIndexes(buf, -1)
	o := 0
	for _, idx := range padIndexes {
		_, err := ti.tty.Write(buf[o:idx[0]])
		if err != nil {
			return err
		}
		n, err := strconv.Atoi(string(buf[idx[2]:idx[3]]))
		if err != nil {
			return err
		}
		if idx[4] != -1 {
			n *= affcnt
		}
		ti.pad(n, idx[6] != -1)
		o = idx[1]
	}
	_, err = ti.tty.Write(buf[o:])
	return err
}

func load1(path string) (*TermInfo, error) {
	term := os.Getenv("TERM")
	if term == "" {
		return nil, ErrNoTerm
	}

	filename := filepath.Join(path, term[:1], term)

	tif, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	// from term(5):
	// The header section begins the file.   This  section  contains
	// six  short  integers  in  the  format described below.  These
	// integers are
	//
	//      (1) the magic number (octal 0432);
	//
	//      (2) the size, in bytes, of the names section;
	//
	//      (3) the number of bytes in the boolean section;
	//
	//      (4) the number of short integers in the numbers section;
	//
	//      (5) the  number  of  offsets  (short  integers)  in  the
	//      strings section;
	//
	//      (6) the size, in bytes, of the string table.
	var header [6]int16
	if err := binary.Read(tif, binary.LittleEndian, header[:]); err != nil {
		return nil, &ErrBadThing{Thing: "header", Filename: filename, Err: err}
	}

	isBig := false
	switch header[0] {
	case 0432:
		// ok
	case 01036:
		isBig = true
	default:
		err := fmt.Errorf("expected 0432 or 01036, got %#o", header[0])
		return nil, &ErrBadThing{Thing: "magic", Filename: filename, Err: err}
	}

	rawNames := make([]byte, header[1])
	if _, err := io.ReadFull(tif, rawNames); err != nil {
		return nil, &ErrBadThing{Thing: "names section", Filename: filename, Err: err}
	}

	if rawNames[header[1]-1] != 0 {
		err := errors.New("missing null at end of string")
		return nil, &ErrBadThing{Thing: "names section", Filename: filename, Err: err}
	}

	ti := &TermInfo{
		Names: strings.Split(string(rawNames[:header[1]-1]), "|"),
	}

	rawBools := make([]byte, header[2])
	if _, err := io.ReadFull(tif, rawBools); err != nil {
		return nil, &ErrBadThing{Thing: "booleans section", Filename: filename, Err: err}
	}

	ti.Booleans = make([]bool, header[2])
	for i, b := range rawBools {
		// “This byte is either 0 or 1 as the flag is present
		// or absent”, but empirically it's the other way
		// around.
		ti.Booleans[i] = b == 1
	}

	// “Between the boolean section and the number section, a null
	// byte will be inserted, if necessary, to ensure that the
	// number section begins on an even byte (this is a relic of
	// the PDP-11's word-addressed architecture, originally
	// designed in to avoid IOT traps induced by addressing a word
	// on an odd byte boundary).  All short integers are aligned on
	// a short word boundary.”
	tif.Seek(int64((header[1]+header[2])&1), 1)

	if isBig {
		ti.BigNumbers = make([]int32, header[3])
		if err := binary.Read(tif, binary.LittleEndian, ti.BigNumbers); err != nil {
			return nil, &ErrBadThing{Thing: "numbers section", Filename: filename, Err: err}
		}
	} else {
		ti.Numbers = make([]int16, header[3])
		if err := binary.Read(tif, binary.LittleEndian, ti.Numbers); err != nil {
			return nil, &ErrBadThing{Thing: "numbers section", Filename: filename, Err: err}
		}
	}

	strIndexes := make([]int16, header[4])
	if err := binary.Read(tif, binary.LittleEndian, strIndexes); err != nil {
		return nil, &ErrBadThing{Thing: "strings section", Filename: filename, Err: err}
	}

	strTable := make([]byte, header[5])
	if _, err := io.ReadFull(tif, strTable); err != nil {
		return nil, &ErrBadThing{Thing: "strings table", Filename: filename, Err: err}
	}

	ti.Strings = make(map[StringIndex][]byte)
	for i, idx := range strIndexes {
		if idx == -1 {
			continue
		}
		var w int16
		for j, b := range strTable[idx:] {
			if b == 0 {
				w = int16(j)
				break
			}
		}
		ti.Strings[StringIndex(i)] = strTable[idx : idx+w]
	}

	// TODO: ncurses extended caps?

	return ti, nil
}

func appendSearchPath(path []string, items ...string) []string {
outer:
	for _, it := range items {
		if it == "" {
			it = "/etc/terminfo"
		}
		for _, p := range path {
			if it == p {
				continue outer
			}
		}
		path = append(path, it)
	}

	return path
}

func searchPath() []string {
	// from terminfo(5):

	// Fetching Compiled Descriptions
	//
	// The ncurses library searches for  terminal  descriptions  in  several
	// places.  It uses only the first description found.  The library has a
	// compiled-in list of places to search which can be overridden by envi-
	// ronment  variables.   Before  starting  to search, ncurses eliminates
	// duplicates in its search list.
	//
	// o   If the environment variable TERMINFO is set, it is interpreted as
	//     the  pathname  of a directory containing the compiled description
	//     you are working on.  Only that directory is searched.
	//
	// o   If TERMINFO is not set, ncurses will instead look in  the  direc-
	//     tory $HOME/.terminfo for a compiled description.

	var path []string

	tiDir := os.Getenv("TERMINFO")
	if tiDir == "" {
		tiDir = filepath.Join(home(), ".terminfo")
	}

	path = appendSearchPath(path, tiDir)

	// o   Next,  if  the environment variable TERMINFO_DIRS is set, ncurses
	//     will interpret the contents of that variable as a list of  colon-
	//     separated directories (or database files) to be searched.
	//
	//     An  empty  directory  name  (i.e., if the variable begins or ends
	//     with a colon, or contains adjacent colons) is interpreted as  the
	//     system location /etc/terminfo.

	path = appendSearchPath(path, filepath.SplitList(os.Getenv("TERMINFO_DIRS"))...)

	// o   Finally, ncurses searches these compiled-in locations:
	//
	//     o   a list of directories (no default value), and
	//
	//     o   the system terminfo directory, /etc/terminfo (the compiled-in
	//         default).

	path = appendSearchPath(path, CompiledInLocations...)
	path = appendSearchPath(path, "/etc/terminfo")

	return path
}

func LoadF(tty *os.File) (ti *TermInfo, err error) {
	for _, p := range searchPath() {
		if ti, err = load1(p); err == nil {
			ti.tty = tty
			break
		}
	}

	return ti, err
}

func Load() (ti *TermInfo, err error) {
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		tty = os.Stdout
	}

	return LoadF(tty)
}
