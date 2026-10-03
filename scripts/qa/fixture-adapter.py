#!/usr/bin/env python3
import json, os, sys, time

args = sys.argv[1:]
if args[1:] == ["whoami"]:
    if os.environ.get("AZPIPE_FIXTURE_MODE") == "identity-error":
        sys.exit(1)
    if os.environ.get("AZPIPE_FIXTURE_MODE") == "slow-identity":
        time.sleep(1.5)
    print(json.dumps({"authenticatedUser": {"properties": {"Account": {"$value": "fixture@example.test"}}}}))
    sys.exit(0)
def flag(name):
    return args[args.index(name) + 1]
if flag("--http-method") != "GET":
    sys.exit("fixture refuses mutations")
area, resource = flag("--area"), flag("--resource")
with open(os.environ["AZPIPE_QA_CALL_LOG"], "a") as output:
    output.write(area + "/" + resource + "\n")
if (area, resource) == ("core", "projects"):
    if os.environ.get("AZPIPE_FIXTURE_MODE") == "slow-projects":
        time.sleep(2)
    print(json.dumps({"value": [{"id": "id-%02d" % i, "name": "project-%02d" % i} for i in range(35)]}))
elif (area, resource) == ("build", "definitions"):
    if os.environ.get("AZPIPE_FIXTURE_MODE") == "slow-pipelines":
        time.sleep(2)
    print(json.dumps({"value": [{"id": 202, "name": "fixture pipeline", "path": "\\fixture", "repository": {"name": "fixture-repo"}}]}))
else:
    sys.exit("unsupported fixture read")
