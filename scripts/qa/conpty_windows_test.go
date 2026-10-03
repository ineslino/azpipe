package qa

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type console struct {
	input   *os.File
	output  *os.File
	pc      windows.Handle
	process windows.Handle
	mu      sync.Mutex
	raw     bytes.Buffer
}

func newConsole(t *testing.T, binary string, args []string, width, height int16) *console {
	t.Helper()
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		inR.Close()
		inW.Close()
		t.Fatal(err)
	}
	defer inR.Close()
	defer outW.Close()
	c := &console{input: inW, output: outR}
	if err = windows.CreatePseudoConsole(windows.Coord{X: width, Y: height}, windows.Handle(inR.Fd()), windows.Handle(outW.Fd()), 0, &c.pc); err != nil {
		inR.Close()
		inW.Close()
		outR.Close()
		outW.Close()
		t.Fatal(err)
	}
	go func() {
		data := make([]byte, 8192)
		for {
			n, err := outR.Read(data)
			if n > 0 {
				c.mu.Lock()
				c.raw.Write(data[:n])
				c.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		if c.process != 0 {
			status, _ := windows.WaitForSingleObject(c.process, 0)
			if status == uint32(windows.WAIT_TIMEOUT) {
				windows.TerminateProcess(c.process, 1)
				windows.WaitForSingleObject(c.process, 2000)
			}
		}
		// Keep the output drain alive while ConPTY shuts down, including startup errors.
		windows.ClosePseudoConsole(c.pc)
		c.input.Close()
		c.output.Close()
		if c.process != 0 {
			windows.CloseHandle(c.process)
		}
	})
	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		t.Fatal(err)
	}
	defer attrs.Delete()
	// HPCON is an opaque pointer. UpdateProcThreadAttribute expects its value,
	// rather than the address of the Go variable containing it.
	pcValue := *(*unsafe.Pointer)(unsafe.Pointer(&c.pc))
	if err = attrs.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, pcValue, unsafe.Sizeof(c.pc)); err != nil {
		t.Fatal(err)
	}
	si := &windows.StartupInfoEx{StartupInfo: windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{}))}, ProcThreadAttributeList: attrs.List()}
	line, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{binary}, args...)))
	if err != nil {
		t.Fatal(err)
	}
	pi := &windows.ProcessInformation{}
	if err = windows.CreateProcess(nil, line, nil, nil, false, windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_UNICODE_ENVIRONMENT, nil, nil, &si.StartupInfo, pi); err != nil {
		t.Fatal(err)
	}
	inR.Close()
	outW.Close()
	windows.CloseHandle(pi.Thread)
	c.process = pi.Process
	return c
}

var vt = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z~]|\x1b\][^\x07]*(?:\x07|\x1b\\)`)

func (c *console) expect(t *testing.T, label string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		found := bytes.Contains(vt.ReplaceAll(c.raw.Bytes(), nil), []byte(label))
		c.mu.Unlock()
		if found {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	t.Fatalf("ConPTY did not display %q:\n%s", label, vt.ReplaceAll(c.raw.Bytes(), nil))
}

func (c *console) send(t *testing.T, keys string) {
	t.Helper()
	if _, err := io.WriteString(c.input, keys); err != nil {
		t.Fatal(err)
	}
	time.Sleep(120 * time.Millisecond)
}

func (c *console) finish(t *testing.T, name string) {
	t.Helper()
	c.send(t, "\x1b[200~:q\x1b[201~\r")
	status, err := windows.WaitForSingleObject(c.process, 5000)
	if err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatalf("ConPTY quit: status=%v err=%v", status, err)
	}
	var code uint32
	if err := windows.GetExitCodeProcess(c.process, &code); err != nil || code != 0 {
		t.Fatalf("ConPTY exit=%d err=%v", code, err)
	}
	if dir := os.Getenv("AZPIPE_QA_CAPTURE_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		if err := os.WriteFile(filepath.Join(dir, name+".ansi"), c.raw.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNativeConPTY(t *testing.T) {
	binary := os.Getenv("AZPIPE_TEST_BINARY")
	if binary == "" {
		t.Skip("set AZPIPE_TEST_BINARY to the built native executable")
	}
	for _, env := range []string{"AZDO_PAT", "AZDO_ORG", "AZPIPE_AZDO_AS", "AZPIPE_AUTH_PROFILE", "AZPIPE_EXPECTED_IDENTITY", "AZPIPE_CONTRACTS"} {
		t.Setenv(env, "")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("AZPIPE_DATA_DIR", filepath.Join(home, "data"))
	for _, size := range [][2]int16{{60, 24}, {80, 24}, {120, 40}} {
		for _, plain := range []bool{false, true} {
			name := fmt.Sprintf("%dx%d-plain-%t", size[0], size[1], plain)
			t.Run(name, func(t *testing.T) {
				if plain {
					t.Setenv("NO_COLOR", "1")
				} else {
					t.Setenv("NO_COLOR", "")
				}
				t.Run("welcome", func(t *testing.T) {
					c := newConsole(t, binary, []string{"--org", "fixture-org"}, size[0], size[1])
					c.expect(t, "Organização:")
					c.expect(t, "AZPIPE")
					c.finish(t, "welcome-"+name)
				})
				t.Run("catalog", func(t *testing.T) {
					c := newConsole(t, binary, []string{"demo"}, size[0], size[1])
					c.expect(t, "PIPELINES")
					c.send(t, " ")
					c.expect(t, "[x]")
					c.send(t, "e")
					c.expect(t, "Ambiente")
					c.send(t, "\x1bOQ")
					c.expect(t, "Enter escolhe")
					c.send(t, "\x1b")
					c.send(t, "\x1b")
					c.send(t, "\r")
					c.expect(t, "SHA:")
					c.send(t, "\r")
					c.expect(t, "MONITORIZAÇÃO")
					c.send(t, "\x1b")
					if err := windows.ResizePseudoConsole(c.pc, windows.Coord{X: 80, Y: 24}); err != nil {
						t.Fatal(err)
					}
					c.finish(t, "catalog-"+name)
				})
				t.Run("branches", func(t *testing.T) {
					c := newConsole(t, binary, []string{"branches", "--demo"}, size[0], size[1])
					c.expect(t, "GESTÃO DE BRANCHES")
					c.send(t, "\x1b[B ")
					c.send(t, "\r")
					c.expect(t, "SHA:")
					c.finish(t, "branches-"+name)
				})
			})
		}
	}
}
