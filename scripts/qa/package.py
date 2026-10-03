#!/usr/bin/env python3
"""Build a local archive and verify its checksum and extracted executable."""
import hashlib, os, pathlib, subprocess, sys, tarfile, zipfile

binary, target, destination = pathlib.Path(sys.argv[1]), sys.argv[2], pathlib.Path(sys.argv[3])
destination.mkdir(parents=True, exist_ok=True)
windows = target.startswith("windows-")
archive = destination / ("azpipe_qa_" + target + (".zip" if windows else ".tar.gz"))
name = "azpipe.exe" if windows else "azpipe"
if windows:
    with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_DEFLATED) as output:
        output.write(binary, name)
else:
    with tarfile.open(archive, "w:gz") as output:
        output.add(binary, arcname=name)
digest = hashlib.sha256(archive.read_bytes()).hexdigest()
(destination / "checksums.txt").write_text(digest + "  " + archive.name + "\n")
assert hashlib.sha256(archive.read_bytes()).hexdigest() == digest
extracted = destination / "extracted"
extracted.mkdir(exist_ok=True)
if windows:
    with zipfile.ZipFile(archive) as source: source.extractall(extracted)
else:
    with tarfile.open(archive) as source: source.extractall(extracted, filter="data")
installed = extracted / name
assert hashlib.sha256(installed.read_bytes()).digest() == hashlib.sha256(binary.read_bytes()).digest()
subprocess.run([str(installed.resolve()), "--version"], check=True)
print("PASS: archive, SHA-256, extracted executable")
