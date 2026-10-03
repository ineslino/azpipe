#!/usr/bin/env python3
"""Native Unix PTY checks; synthetic adapter refuses remote mutations."""
import codecs, fcntl, json, os, pathlib, pty, re, select, signal, struct, sys, tempfile, termios, time, unicodedata

binary = str(pathlib.Path(sys.argv[1]).resolve())
captures = pathlib.Path(os.environ.get("AZPIPE_QA_CAPTURE_DIR", "dist/qa-captures"))
captures.mkdir(parents=True, exist_ok=True)
results = []
selected = set(sys.argv[2:])
os.environ["AZPIPE_QA_CALL_LOG"] = str(captures.resolve() / "adapter-calls.txt")
(captures / "adapter-calls.txt").write_text("")

class Terminal:
    def __init__(self, name, args, width=80, height=24, mode="", plain=False):
        self.name, self.width, self.height = name, width, height
        self.lines = [[" "] * width for _ in range(height)]
        self.x = self.y = 0
        self.pending = ""
        self.raw = bytearray()
        self.data = pathlib.Path(tempfile.mkdtemp(prefix="isolated-data-"+name+"-", dir=captures.resolve()))
        (captures / "isolated-home").mkdir(exist_ok=True)
        self.decoder = codecs.getincrementaldecoder("utf-8")("replace")
        self.pid, self.fd = pty.fork()
        if self.pid == 0:
            env = dict(os.environ, HOME=str(captures.resolve() / "isolated-home"), AZPIPE_DATA_DIR=str(self.data), TERM="xterm-256color", COLORTERM="truecolor", AZDO_PAT="", AZPIPE_CONTRACTS="", AZPIPE_AZDO_AS=str(pathlib.Path(__file__).with_name("fixture-adapter.py").resolve()), AZPIPE_AUTH_PROFILE="fixture", AZPIPE_EXPECTED_IDENTITY="fixture@example.test", AZPIPE_FIXTURE_MODE=mode)
            env.pop("NO_COLOR", None)
            env["COLORFGBG"] = "0;15" if os.environ.get("AZPIPE_QA_LIGHT") else "15;0"
            if plain: env["NO_COLOR"] = "1"
            os.execve(binary, [binary] + args, env)
        fcntl.ioctl(self.fd, termios.TIOCSWINSZ, struct.pack("HHHH", height, width, 0, 0))
        self.done = False
        self.read(1.0)
    def parse(self, text):
        data = self.pending + text
        self.pending = ""
        i = 0
        while i < len(data):
            c = data[i]
            if c == "\x1b":
                match = re.match(r"\x1b\[([0-9;?]*)([A-Za-z~])|\x1b\][^\x07]*(?:\x07|\x1b\\)|\x1b[()][A-Za-z0-9]|\x1b[=>78]", data[i:])
                if not match:
                    self.pending = data[i:]
                    break
                params, command = match.group(1), match.group(2)
                if command:
                    values = [int(n) if n else 0 for n in (params or "").lstrip("?").split(";")]
                    n = values[0] or 1
                    if command in ("H", "f"):
                        self.y, self.x = min(self.height-1, n-1), min(self.width-1, (values[1] if len(values)>1 and values[1] else 1)-1)
                    elif command == "A": self.y = max(0, self.y-n)
                    elif command == "B": self.y = min(self.height-1, self.y+n)
                    elif command == "C": self.x = min(self.width-1, self.x+n)
                    elif command == "D": self.x = max(0, self.x-n)
                    elif command == "G": self.x = min(self.width-1, n-1)
                    elif command == "K":
                        start, end = (0, self.width) if values[0] == 2 else (0, self.x+1) if values[0] == 1 else (self.x, self.width)
                        self.lines[self.y][start:end] = [" "] * (end-start)
                    elif command == "J":
                        if values[0] == 2: self.lines = [[" "]*self.width for _ in range(self.height)]
                        else:
                            self.lines[self.y][self.x:] = [" "]*(self.width-self.x)
                            for row in range(self.y+1, self.height): self.lines[row] = [" "]*self.width
                    elif command == "n" and n == 6: os.write(self.fd, b"\x1b[1;1R")
                i += len(match.group(0))
                continue
            if c == "\r": self.x = 0
            elif c == "\n":
                self.y += 1
                if self.y >= self.height:
                    self.lines.pop(0); self.lines.append([" "]*self.width); self.y = self.height-1
            elif c == "\b": self.x = max(0, self.x-1)
            elif ord(c) >= 32 and not unicodedata.combining(c):
                if self.x >= self.width: self.x, self.y = 0, min(self.height-1, self.y+1)
                self.lines[self.y][self.x] = c
                self.x += 2 if unicodedata.east_asian_width(c) in ("W", "F") else 1
            i += 1
    def read(self, duration=.25):
        deadline = time.monotonic()+duration
        while time.monotonic() < deadline:
            readable, _, _ = select.select([self.fd], [], [], min(.05, max(0, deadline-time.monotonic())))
            if readable:
                try: chunk = os.read(self.fd, 65536)
                except OSError: break
                if not chunk: break
                self.raw.extend(chunk); self.parse(self.decoder.decode(chunk))
    def resize(self, width, height):
        self.width, self.height = width, height
        self.lines = [[" "] * width for _ in range(height)]
        self.x = self.y = 0
        fcntl.ioctl(self.fd, termios.TIOCSWINSZ, struct.pack("HHHH", height, width, 0, 0))
        os.kill(self.pid, signal.SIGWINCH)
        self.read(.4)
    def send(self, text, duration=.25):
        os.write(self.fd, text.encode()); self.read(duration)
    def capture(self, suffix, required=(), absent=()):
        deadline = time.monotonic()+5
        while True:
            view = "\n".join("".join(row).rstrip() for row in self.lines).rstrip()
            if all(label in view for label in required) or time.monotonic() >= deadline:
                break
            self.read(.1)
        for label in required: assert label in view, (self.name, suffix, "missing", label, view)
        for label in absent: assert label not in view, (self.name, suffix, "unexpected", label, view)
        (captures / (self.name+"-"+suffix+".txt")).write_text(view+"\n")
        return view
    def finish(self, command=":q", paste=False):
        self.send(("\x1b[200~"+command+"\x1b[201~") if paste else command)
        self.send("\r", .4)
        deadline = time.monotonic()+2
        while time.monotonic() < deadline:
            pid, status = os.waitpid(self.pid, os.WNOHANG)
            if pid:
                self.done = True
                assert os.waitstatus_to_exitcode(status) == 0, (self.name, status)
                break
            self.read(.1)
        assert self.done, (self.name, "did not exit")
        (captures / (self.name+".ansi")).write_bytes(self.raw)
        os.close(self.fd)
        results.append({"scenario":self.name, "result":"PASS"})
    def cleanup(self):
        if not self.done:
            os.kill(self.pid, signal.SIGTERM)
            os.waitpid(self.pid, 0)
            os.close(self.fd)

def scenario(name, args, task, **kwargs):
    if selected and name not in selected: return
    terminal = Terminal(name, args, **kwargs)
    try: task(terminal)
    finally: terminal.cleanup()

def context_flow(t):
    t.capture("welcome", ("AZPIPE", "Organização:"))
    t.send("\r", .8)
    t.capture("projects", ("35 projectos", "Procurar:"))
    t.send("/id-34\r")
    t.capture("filtered", ("project-34", "1–1 / 1"), ("project-00",))
    t.send("\r", .8)
    t.capture("pipeline", ("fixture pipeline",))
    t.finish(":q", True)

def catalog_flow(t):
    t.capture("catalog", ("AZPIPE", "PIPELINES"))
    t.send(" ")
    t.capture("selected", ("[x]",))
    t.send("/")
    t.send("build")
    t.send("\x1b", .35)
    t.capture("filter-kept", ("build", "1–1 / 1"))
    t.send("/")
    t.send("\x15\r")
    t.send("a")
    t.capture("menu-top", ("SELECCIONAR",))
    t.send("\x1b[B" * 13)
    t.capture("menu-bottom", ("Gerir branches",))
    t.send("e", .4)
    t.capture("parameters", ("Ambiente", "Campo 1 de 3", "Ctrl+S aplica apenas", "Opções:"))
    t.send("\x1bOQ")  # F2 opens choices without changing the default.
    t.capture("options", ("Opções", "staging", "Enter escolhe"))
    t.send("\x1b[B\x1b", .35)
    t.capture("options-cancelled", ("predefinido pela pipeline",))
    t.send("\t\t")
    t.send("\x15not-a-number\x13")
    t.capture("parameter-error", ("Réplicas", "número", "PgUp/PgDn"))
    t.send("\x1b", .35)
    t.send("l")
    t.capture("profiles-empty", ("Nenhum registo", "perfil"), ("enter carregar",))
    t.send("\x1b", .35)
    t.send("h")
    t.capture("history", ("build application", "Data indisponível"))
    t.send("d")
    t.capture("history-detail", ("build application", "deploy infrastructure", "release website"))
    t.send("\x1b", .35)
    t.send("\r")
    t.capture("monitor-history", ("MONITORIZAÇÃO",))
    t.send("\x1b", .35)
    t.send("\r", .5)
    required = ("SHA:", "Revisão")
    if t.height >= 40:
        required += ("Definição:", "Valores predefinidos:")
    t.capture("review", required)
    t.send("\r", .4)
    t.capture("monitor", ("MONITORIZAÇÃO",))
    t.send("\x1b", .35)
    t.resize(80, 24)
    t.capture("resized", ("AZPIPE", "[x]"))
    t.finish(":q", True)

def branch_flow(t):
    t.capture("branches", ("GESTÃO DE BRANCHES",))
    t.send("\x1b[B ")
    t.send("\r")
    t.capture("branch-review", ("SHA:", "Eliminação indisponível"))
    t.finish(":q", True)

def long_options(t):
    t.send("\r", .8)
    t.send("/id-00\r")
    t.send("\r", .8)
    t.send("e", .8)
    t.capture("parameters", ("Ambiente", "predefinido pela pipeline"))
    t.send("\x1bOQ")
    t.capture("options", ("Valor completo", "PgUp/PgDn detalhe"))
    t.send("\x1b[6~"*5)
    t.capture("test-suffix", ("test",))
    t.send("\x1b[B")
    t.send("\x1b[6~"*5)
    t.capture("prod-suffix", ("prod",))
    t.send("\x1b", .35)
    t.capture("cancelled", ("predefinido pela pipeline",))
    t.send("\x1bOQ\x1b[B\r")
    t.capture("chosen", ("valor personalizado",))
    t.send("\x1bOQ")
    t.send("\x1b[6~"*5)
    t.capture("chosen-detail", ("prod",))
    t.send("\x1b", .35)
    t.send("\x13")
    t.send("e", .8)
    t.capture("saved", ("valor personalizado",))
    t.send("\x1bOQ")
    t.send("\x1b[6~"*5)
    t.capture("saved-detail", ("prod",))
    t.send("\x1b", .35)
    t.send("\x1b", .35)
    t.finish(":q", True)

def cancellation(t):
    t.send("\r", .25)
    t.send("\x1b", .35)
    t.capture("cancelled", ("Leitura cancelada",))
    t.read(2)
    t.capture("late-discarded", ("Leitura cancelada",), ("35 projectos",))
    t.finish(":q", True)

def recovery(t):
    t.send("\r", .5)
    t.capture("error", ("ERRO", "COMO RECUPERAR", "enter ligar"))
    t.finish(":q", True)

def long_context(t):
    t.send("\r", .8)
    t.send("\r", .8)
    t.capture("catalog-target", ("Projecto: project-00", "fixture pipeline"))
    t.send(" ")
    t.send("\r", .8)
    t.capture("review-target", ("Projecto: project-00", "bloqueada"))
    t.finish(":q", True)

def fixture_catalog(t):
    t.send("\r", .8)
    t.send("\r", .8)
    t.capture("catalog", ("fixture pipeline", "Projecto: project-00"))

def context_retention(t):
    fixture_catalog(t)
    t.send(" ")
    t.send("e", .8)
    t.send("\x17\x13")  # Ctrl+W edits a default; Ctrl+S must send that visible value.
    t.send("e", .8)
    t.capture("edited-default", ("valor personalizado", "Valor: alpha"), ("beta",))
    t.send("\x1b", .35)
    t.send("b\x15release/prepared\r")
    t.send("/fixture\x1b", .35)
    t.send("c")
    t.capture("scope-consequence", ("Outro contexto limpa",))
    t.send("\r", .6)
    t.capture("retained", ("[x]", "release/prepared", "Procurar: fixture", "Contexto mantido"))
    t.send("e", .8)
    t.capture("retained-parameter", ("valor personalizado", "Valor: alpha"), ("beta",))
    t.send("\x1b", .35)
    t.send("c\x1b[B\r", .8)
    t.capture("new-context", ("Projecto: project-01", "Branch: main", "[ ]"), ("[x]", "release/prepared", "Contexto mantido"))
    t.finish(":q", True)

def schema_cancellation(t):
    fixture_catalog(t)
    t.send("e", .25)
    t.send("\x1b", .35)
    t.capture("cancelled", ("Leitura de parâmetros cancelada",), ("CONFIGURAR PARÂMETROS",))
    t.read(2)
    t.capture("late-discarded", ("Leitura de parâmetros cancelada",), ("CONFIGURAR PARÂMETROS",))
    t.send("e", .25)
    t.capture("reopened", ("Etiqueta", "Valor: alpha beta"))
    t.send("\x1b", .35)
    t.finish(":q", True)

def profile_recovery(t):
    fixture_catalog(t)
    directory = t.data / "profiles"
    directory.mkdir(parents=True, mode=0o700)
    for name, pipeline in (("a-removed", 9999), ("b-valid", 202)):
        profile = {"version": 1, "name": name, "organization": "fixture-org", "project": "project-00", "selections": [{"id": pipeline, "project": "project-00", "mode": "RUN", "branch": "main"}]}
        fd = os.open(directory / (name+".json"), os.O_WRONLY|os.O_CREAT|os.O_EXCL, 0o600)
        with os.fdopen(fd, "w") as output: json.dump(profile, output)
    t.send(" ")
    t.send("l\r")
    t.capture("invalid-profile", ("a-removed", "9999", "Escolhe outro perfil", "↑/↓ escolher", "enter carregar"))
    t.send("\x1b[B\r")
    t.capture("valid-profile", ("Perfil carregado", "[x]"), ("a-removed", "9999"))
    t.finish(":q", True)

for width, height, plain in ((60, 24, False), (80, 24, False), (120, 40, True)):
    suffix = str(width) + ("-plain" if plain else "")
    scenario("context-"+suffix, ["--org", "fixture-org", "--project", "project-00"], context_flow, width=width, height=height, plain=plain)
    scenario("catalog-"+suffix, ["demo"], catalog_flow, width=width, height=height, plain=plain)
    scenario("branches-"+suffix, ["branches", "--demo"], branch_flow, width=width, height=height, plain=plain)
scenario("cancel-projects", ["--org", "fixture-org"], cancellation, mode="slow-projects")
scenario("auth-recovery", ["--org", "fixture-org"], recovery, mode="identity-error")
scenario("long-context-60", ["--org", "organization"*12, "--project", "project-00"], long_context, width=60, height=24)
scenario("long-options-60", ["--org", "fixture-org", "--project", "project-00"], long_options, mode="long-options", width=60, height=24)
scenario("context-retain-80", ["--org", "fixture-org", "--project", "project-00"], context_retention, mode="continuity")
scenario("cancel-schema-80", ["--org", "fixture-org", "--project", "project-00"], schema_cancellation, mode="slow-schema")
scenario("profile-recovery-80", ["--org", "fixture-org", "--project", "project-00"], profile_recovery)
print(json.dumps(results))
