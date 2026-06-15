#!/usr/bin/env python3
"""Example script for testing external script support in formula."""
import json
import os
import sys

def main():
    # Read environment variables
    name = os.environ.get("TT_NAME", "world")
    greeting = os.environ.get("TT_GREETING", "Hello")
    
    # Output JSON result
    result = {
        "ok": True,
        "message": f"{greeting}, {name}!",
        "source": "external_script",
        "cwd": os.getcwd()
    }
    
    print(json.dumps(result, ensure_ascii=False))

if __name__ == "__main__":
    main()
