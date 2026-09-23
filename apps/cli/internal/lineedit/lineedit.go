// Package lineedit is a small line editor for the vrx REPL: raw terminal mode, emacs-style editing keys, history
// (↑/↓, Ctrl-P/N), Tab completion and `?` help. It uses only golang.org/x/sys/unix (already in the repo's Go
// dependency set) instead of a third-party readline package. When the input is not a terminal it reads plain lines.
package lineedit

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

// ErrInterrupt is returned on Ctrl-C (the caller discards the line and continues).
var ErrInterrupt = errors.New("interrupt")

// Candidate is one completion.
type Candidate struct {
	Text string // replacement for the word being completed
	Help string // one-line description (shown in lists and `?`)
	More bool   // true: the word is a prefix of longer input (no trailing space after completing it)
}

// Completer returns the completions of the word that ends at the end of line and the byte offset where that word
// starts.
type Completer func(line string) (cands []Candidate, wordStart int)

// Helper returns the text printed when the user types `?` (the help of the position at the end of line).
type Helper func(line string) string

// Editor reads lines.
type Editor struct {
	In       *os.File
	Out      io.Writer
	Complete Completer
	Help     Helper
	History  []string
	MaxHist  int

	reader *bufio.Reader
}

// IsTerminal reports whether f is a terminal.
func IsTerminal(f *os.File) bool {
	_, err := unix.IoctlGetTermios(int(f.Fd()), unix.TCGETS)
	return err == nil
}

// AddHistory appends a line (no duplicates of the previous line, no blank lines).
func (e *Editor) AddHistory(line string) {
	line = strings.TrimSpace(line)
	if line == "" || (len(e.History) > 0 && e.History[len(e.History)-1] == line) {
		return
	}
	e.History = append(e.History, line)
	if max := e.maxHist(); len(e.History) > max {
		e.History = e.History[len(e.History)-max:]
	}
}

func (e *Editor) maxHist() int {
	if e.MaxHist > 0 {
		return e.MaxHist
	}
	return 1000
}

// ReadLine prints prompt and returns one line (without the newline). io.EOF on Ctrl-D at an empty line or end of
// input; ErrInterrupt on Ctrl-C.
func (e *Editor) ReadLine(prompt string) (string, error) {
	fd := int(e.In.Fd())
	old, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return e.readPlain()
	}
	raw := *old
	raw.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	raw.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	raw.Cflag &^= unix.CSIZE | unix.PARENB
	raw.Cflag |= unix.CS8
	raw.Cc[unix.VMIN] = 1
	raw.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, &raw); err != nil {
		return e.readPlain()
	}
	defer func() { _ = unix.IoctlSetTermios(fd, unix.TCSETS, old) }()
	return e.edit(prompt)
}

func (e *Editor) rd() *bufio.Reader {
	if e.reader == nil {
		e.reader = bufio.NewReader(e.In)
	}
	return e.reader
}

func (e *Editor) readPlain() (string, error) {
	line, err := e.rd().ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

type state struct {
	e      *Editor
	prompt string
	buf    []rune
	pos    int
	hist   int    // index into history while browsing; len(History) = the line being edited
	saved  []rune // the line being edited while browsing history
	in     *bufio.Reader
}

func (e *Editor) edit(prompt string) (string, error) {
	s := &state{e: e, prompt: prompt, hist: len(e.History), in: e.rd()}
	s.refresh()
	for {
		r, _, err := s.in.ReadRune()
		if err != nil {
			return "", err
		}
		switch r {
		case '\r', '\n':
			s.write("\r\n")
			return string(s.buf), nil
		case 3: // Ctrl-C
			s.write("^C\r\n")
			return "", ErrInterrupt
		case 4: // Ctrl-D
			if len(s.buf) == 0 {
				s.write("\r\n")
				return "", io.EOF
			}
			s.deleteAt()
		case 1: // Ctrl-A
			s.pos = 0
		case 5: // Ctrl-E
			s.pos = len(s.buf)
		case 2: // Ctrl-B
			if s.pos > 0 {
				s.pos--
			}
		case 6: // Ctrl-F
			if s.pos < len(s.buf) {
				s.pos++
			}
		case 8, 127: // Backspace
			if s.pos > 0 {
				s.buf = append(s.buf[:s.pos-1], s.buf[s.pos:]...)
				s.pos--
			}
		case 11: // Ctrl-K
			s.buf = s.buf[:s.pos]
		case 21: // Ctrl-U
			s.buf = append([]rune{}, s.buf[s.pos:]...)
			s.pos = 0
		case 23: // Ctrl-W
			i := s.pos
			for i > 0 && s.buf[i-1] == ' ' {
				i--
			}
			for i > 0 && s.buf[i-1] != ' ' {
				i--
			}
			s.buf = append(s.buf[:i], s.buf[s.pos:]...)
			s.pos = i
		case 12: // Ctrl-L
			s.write("\x1b[H\x1b[2J")
		case 16: // Ctrl-P
			s.history(-1)
		case 14: // Ctrl-N
			s.history(1)
		case '\t':
			s.complete()
		case '?':
			if s.inQuotes() || e.Help == nil {
				s.insert(r)
			} else {
				text := e.Help(string(s.buf[:s.pos]))
				s.write("?\r\n" + strings.ReplaceAll(strings.TrimRight(text, "\n"), "\n", "\r\n") + "\r\n")
			}
		case 27: // escape sequence
			s.escape()
		default:
			if r >= 32 && r != utf8.RuneError {
				s.insert(r)
			}
		}
		s.refresh()
	}
}

func (s *state) write(str string) { _, _ = io.WriteString(s.e.Out, str) }

func (s *state) refresh() {
	s.write("\r" + s.prompt + string(s.buf) + "\x1b[K")
	if back := len(s.buf) - s.pos; back > 0 {
		s.write(fmt.Sprintf("\x1b[%dD", back))
	}
}

func (s *state) insert(r rune) {
	s.buf = append(s.buf[:s.pos], append([]rune{r}, s.buf[s.pos:]...)...)
	s.pos++
}

func (s *state) insertString(str string) {
	for _, r := range str {
		s.insert(r)
	}
}

func (s *state) deleteAt() {
	if s.pos < len(s.buf) {
		s.buf = append(s.buf[:s.pos], s.buf[s.pos+1:]...)
	}
}

func (s *state) inQuotes() bool {
	var q rune
	for _, r := range s.buf[:s.pos] {
		switch {
		case q == 0 && (r == '"' || r == '\''):
			q = r
		case q != 0 && r == q:
			q = 0
		}
	}
	return q != 0
}

func (s *state) history(dir int) {
	h := s.e.History
	if len(h) == 0 {
		return
	}
	if s.hist == len(h) {
		s.saved = append([]rune{}, s.buf...)
	}
	n := s.hist + dir
	if n < 0 || n > len(h) {
		return
	}
	s.hist = n
	if n == len(h) {
		s.buf = append([]rune{}, s.saved...)
	} else {
		s.buf = []rune(h[n])
	}
	s.pos = len(s.buf)
}

func (s *state) escape() {
	b, err := s.in.ReadByte()
	if err != nil {
		return
	}
	if b != '[' && b != 'O' {
		return
	}
	c, err := s.in.ReadByte()
	if err != nil {
		return
	}
	switch c {
	case 'A':
		s.history(-1)
	case 'B':
		s.history(1)
	case 'C':
		if s.pos < len(s.buf) {
			s.pos++
		}
	case 'D':
		if s.pos > 0 {
			s.pos--
		}
	case 'H':
		s.pos = 0
	case 'F':
		s.pos = len(s.buf)
	default:
		if c >= '0' && c <= '9' {
			t, _ := s.in.ReadByte()
			if t != '~' {
				return
			}
			switch c {
			case '1', '7':
				s.pos = 0
			case '4', '8':
				s.pos = len(s.buf)
			case '3':
				s.deleteAt()
			}
		}
	}
}

func (s *state) complete() {
	if s.e.Complete == nil {
		return
	}
	line := string(s.buf[:s.pos])
	cands, start := s.e.Complete(line)
	if len(cands) == 0 {
		s.write("\a")
		return
	}
	word := line[start:]
	if len(cands) == 1 {
		s.replaceWord(len([]rune(word)), cands[0].Text)
		if !cands[0].More {
			s.insert(' ')
		}
		return
	}
	texts := make([]string, len(cands))
	for i, c := range cands {
		texts[i] = c.Text
	}
	if p := commonPrefix(texts); len(p) > len(word) && strings.HasPrefix(p, word) {
		s.replaceWord(len([]rune(word)), p)
		return
	}
	s.write("\r\n" + strings.ReplaceAll(List(cands, 100), "\n", "\r\n"))
}

func (s *state) replaceWord(runes int, with string) {
	from := s.pos - runes
	rest := append([]rune{}, s.buf[s.pos:]...)
	s.buf = s.buf[:from]
	s.pos = from
	s.insertString(with)
	s.buf = append(s.buf, rest...)
}

func commonPrefix(list []string) string {
	if len(list) == 0 {
		return ""
	}
	p := list[0]
	for _, s := range list[1:] {
		for !strings.HasPrefix(s, p) {
			p = p[:len(p)-1]
		}
	}
	return p
}

// List formats candidates as an aligned two-column list (text, help), sorted as given.
func List(cands []Candidate, width int) string {
	w := 0
	for _, c := range cands {
		if l := utf8.RuneCountInString(c.Text); l > w {
			w = l
		}
	}
	if w > 40 {
		w = 40
	}
	var b strings.Builder
	for _, c := range cands {
		if c.Help == "" {
			fmt.Fprintf(&b, "  %s\n", c.Text)
			continue
		}
		help := c.Help
		if max := width - w - 6; max > 10 && utf8.RuneCountInString(help) > max {
			help = string([]rune(help)[:max-1]) + "…"
		}
		fmt.Fprintf(&b, "  %-*s  %s\n", w, c.Text, help)
	}
	return b.String()
}

// SortCandidates sorts by text (stable for equal texts).
func SortCandidates(c []Candidate) {
	sort.SliceStable(c, func(i, j int) bool { return c[i].Text < c[j].Text })
}

// ReadSecret reads one line without echo (passwords) through the editor's input buffer. Not a terminal → plain
// line.
func (e *Editor) ReadSecret(prompt string) (string, error) {
	_, _ = io.WriteString(e.Out, prompt)
	fd := int(e.In.Fd())
	old, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return e.readPlain()
	}
	noEcho := *old
	noEcho.Lflag &^= unix.ECHO
	noEcho.Lflag |= unix.ICANON | unix.ISIG
	noEcho.Iflag |= unix.ICRNL
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, &noEcho); err != nil {
		return "", err
	}
	defer func() {
		_ = unix.IoctlSetTermios(fd, unix.TCSETS, old)
		_, _ = io.WriteString(e.Out, "\n")
	}()
	return e.readPlain()
}
