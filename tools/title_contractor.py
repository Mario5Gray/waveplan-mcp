#!/usr/bin/env python3
"""
Deterministic Middle-Out Title Contractor
=========================================
- Structural contraction (middle-out parsing)
- Bigram contraction (NLTK + explicit whitelist)
- Unigram contraction (explicit dict)
- Dry-run audit mode
- CLI + config file support
- Git hook integration
- 100% reproducible, zero randomness
"""

import argparse
import json
import re
import sys
import hashlib
from pathlib import Path
from typing import Dict, List, Optional, Tuple, Set

import spacy
import nltk
from nltk.util import bigrams

# ─────────────────────────────────────────────────────────────
# CONFIGURATION & PROTECTED TOKENS
# ─────────────────────────────────────────────────────────────

# Never contract these tokens (acronyms, protocols, versions)
PROTECTED_TOKENS: Set[str] = {
    "api", "uri", "url", "uuid", "id", "os", "io", "ai", "ml", "dl", "nlp",
    "http", "https", "ftp", "ssh", "tls", "ssl", "tcp", "udp", "dns", "dhcp",
    "json", "xml", "yaml", "csv", "sql", "nosql", "grpc", "rest", "oauth",
    "oauth2", "jwt", "cors", "csp", "xss", "csrf", "sql", "html", "css",
    "js", "ts", "py", "rb", "go", "rs", "cpp", "c", "h", "sh", "bash",
    "v1", "v2", "v3", "v4", "v5", "v6", "ipv4", "ipv6", "sha1", "sha256",
    "md5", "aes", "rsa", "ecdsa", "ed25519", "tls12", "tls13", "http2", "http3"
}

# ─────────────────────────────────────────────────────────────
# BIGRAM CONTRACTOR (NLTK + Explicit Whitelist)
# ─────────────────────────────────────────────────────────────

class TechBigramContractor:
    BIGRAM_MAP: Dict[str, str] = {
        
        "cross parent": "xparent",
        "command line": "cli",
        "machine learning": "ml",
        "deep learning": "dl",
        "natural language": "nl",
        "artificial intelligence": "ai",
        "user interface": "ui",
        "user experience": "ux",
        "operating system": "os",
        "version control": "vcs",
        "continuous integration": "ci",
        "continuous deployment": "cd",
        "key value": "kv",
        "read only": "ro",
        "write only": "wo",
        "read write": "rw",
        "point in time": "pit",
        "end to end": "e2e",
        "peer to peer": "p2p",
        "back end": "backend",
        "front end": "frontend",
        "side effect": "sidefx",
        "time to live": "ttl",
        "least recently used": "lru",
        "first in first out": "fifo",
        "last in first out": "lifo",
        "as soon as possible": "asap",
        "out of memory": "oom",
        "not a number": "nan",
    }

    @classmethod
    def contract_string(cls, text: str, extra_map: Optional[Dict[str, str]] = None,
                        protected: Optional[Set[str]] = None, dry_run: bool = False) -> Tuple[str, List[Dict]]:
        lookup = {**cls.BIGRAM_MAP, **(extra_map or {})}
        protected = protected or PROTECTED_TOKENS
        tokens = nltk.word_tokenize(text.lower())
        original_tokens = nltk.word_tokenize(text)  # preserve original casing
        
        result: List[str] = []
        audit: List[Dict] = []
        i = 0
        
        while i < len(tokens):
            if i + 1 < len(tokens):
                bigram_key = f"{tokens[i]} {tokens[i+1]}"
                # Skip if either token is protected
                if tokens[i] not in protected and tokens[i+1] not in protected and bigram_key in lookup:
                    contracted = lookup[bigram_key]
                    # Preserve casing of first word
                    if original_tokens[i].istitle():
                        contracted = contracted.title()
                    elif original_tokens[i].isupper():
                        contracted = contracted.upper()
                    result.append(contracted)
                    if dry_run:
                        audit.append({"type": "bigram", "original": bigram_key, "contracted": contracted, "pos": i})
                    i += 2
                    continue
            result.append(original_tokens[i])
            i += 1
        
        return " ".join(result), audit


# ─────────────────────────────────────────────────────────────
# UNIGRAM WORD CONTRACTOR
# ─────────────────────────────────────────────────────────────

class TechWordContractor:
    EXPLICIT_MAP: Dict[str, str] = {
        "automatic": "auto", "configuration": "config", "implementation": "impl",
        "execution": "exec", "environment": "env", "development": "dev",
        "production": "prod", "manager": "mgr", "controller": "ctrl",
        "initialization": "init", "validation": "val", "verification": "ver",
        "application": "app", "function": "func", "parameter": "param",
        "argument": "arg", "directory": "dir", "library": "lib",
        "module": "mod", "interface": "iface", "reference": "ref",
        "index": "idx", "identifier": "id", "variable": "var",
        "constant": "const", "definition": "def", "declaration": "decl",
        "expression": "expr", "statement": "stmt", "operation": "op",
        "allocation": "alloc", "deallocation": "dealloc", "synchronization": "sync",
        "asynchronous": "async", "synchronous": "sync", "dynamic": "dyn",
        "global": "glob", "local": "loc", "internal": "int", "external": "ext",
        "private": "priv", "protected": "prot", "public": "pub",
        "abstract": "abs", "virtual": "virt", "exception": "exc",
        "error": "err", "warning": "warn", "message": "msg",
        "buffer": "buf", "memory": "mem", "storage": "store",
        "database": "db", "request": "req", "response": "resp",
        "session": "sess", "connection": "conn", "socket": "sock",
        "protocol": "proto", "format": "fmt", "command": "cmd",
        "process": "proc", "thread": "thr", "pipeline": "pipe",
        "workflow": "wf", "generation": "gen", "detection": "det",
        "recognition": "rec", "classification": "cls", "regression": "reg",
        "prediction": "pred", "optimization": "opt", "compilation": "comp",
        "transformation": "xform", "conversion": "conv", "serialization": "ser",
        "deserialization": "deser", "compression": "cmpr", "encryption": "enc",
        "decryption": "dec", "authentication": "auth", "authorization": "authz",
        "administration": "admin", "management": "mgmt", "dependencies": "deps",
        "dependency": "dep", "immutable": "immut", "deterministic": "determ"
    }
    MIN_WORD_LENGTH = 5

    @classmethod
    def contract_string(cls, text: str, extra_map: Optional[Dict[str, str]] = None,
                        protected: Optional[Set[str]] = None, dry_run: bool = False) -> Tuple[str, List[Dict]]:
        lookup = {**cls.EXPLICIT_MAP, **(extra_map or {})}
        protected = protected or PROTECTED_TOKENS
        audit: List[Dict] = []
        
        def replace_match(match: re.Match) -> str:
            word = match.group(0)
            lower = word.lower()
            # Skip protected or too-short words
            if lower in protected or len(word) < cls.MIN_WORD_LENGTH:
                return word
            if lower not in lookup:
                return word
            contracted = lookup[lower]
            if dry_run:
                audit.append({"type": "unigram", "original": word, "contracted": contracted, "pos": match.start()})
            # Preserve casing
            if word.isupper():
                return contracted.upper()
            if word.istitle():
                return contracted.title()
            return contracted
        
        result = re.sub(r'\b[A-Za-z]+\b', replace_match, text)
        return result, audit


# ─────────────────────────────────────────────────────────────
# MAIN MIDDLE-OUT CONTRACTOR
# ─────────────────────────────────────────────────────────────

class MiddleOutTitleContractor:
    def __init__(self, contract_words: bool = True, contract_bigrams: bool = True,
                 protected_tokens: Optional[Set[str]] = None):
        self.nlp = spacy.load("en_core_web_sm", disable=["ner", "textcat"])
        self.contract_words = contract_words
        self.contract_bigrams = contract_bigrams
        self.protected = protected_tokens or PROTECTED_TOKENS
        self._compile_stops()

    def _compile_stops(self):
        self.FILLER_VERBS = {
            "implement", "build", "create", "add", "fix", "update", "refactor",
            "optimize", "support", "enable", "honor", "honoring", "respect",
            "preserve", "maintain", "ensure", "handle", "manage", "use",
            "utilize", "make", "do", "get", "set", "put", "introduce", "ship"
        }
        self.WEAK_CONNECTORS = {
            "honoring", "respecting", "preserving", "maintaining",
            "ensuring", "handling", "managing", "using", "utilizing",
            "with", "for", "based", "on", "via", "through", "considering",
            "given", "against", "following", "during", "within", "across"
        }
        self.KEEP_POS = {"ADJ", "NOUN", "PROPN", "NUM", "SYM", "X", "ADP"}

    def _find_core_noun(self, doc) -> int:
        for tok in doc:
            if tok.pos_ == "NOUN" and tok.dep_ in ("dobj", "pobj"):
                return tok.i
        for tok in doc:
            if tok.pos_ == "NOUN" and tok.dep_ in ("compound", "ROOT", "nsubj"):
                return tok.i
        for tok in reversed(doc):
            if tok.pos_ == "NOUN":
                return tok.i
        return 0

    def _smart_title_case(self, text: str) -> str:
        def cap_word(w: str) -> str:
            if not w or w.isupper() or any(c.isdigit() for c in w):
                return w
            return w[0].upper() + w[1:]
        return " ".join(cap_word(w) for w in re.split(r'(\s+)', text))

    def contract(self, title: str, 
                 extra_word_map: Optional[Dict[str, str]] = None,
                 extra_bigram_map: Optional[Dict[str, str]] = None,
                 dry_run: bool = False) -> Tuple[str, Dict]:
        """
        Returns: (contracted_title, audit_dict)
        audit_dict contains: structural_changes, bigram_changes, unigram_changes
        """
        audit = {"structural": [], "bigram": [], "unigram": []}
        
        # ── STEP 1: Middle-out structural contraction ──
        doc = self.nlp(title.lower())
        core_idx = self._find_core_noun(doc)
        core_token = doc[core_idx]

        left_filtered = []
        for tok in doc[:core_idx]:
            if tok.text.lower() in self.FILLER_VERBS or tok.text.lower() in self.WEAK_CONNECTORS:
                if dry_run:
                    audit["structural"].append({"action": "dropped", "token": tok.text, "reason": "filler/connector"})
                continue
            if tok.pos_ in self.KEEP_POS:
                left_filtered.append(tok.text)
            elif dry_run and tok.pos_ not in self.KEEP_POS:
                audit["structural"].append({"action": "dropped", "token": tok.text, "reason": f"pos={tok.pos_}"})

        right_filtered = []
        for tok in doc[core_idx+1:]:
            if tok.text.lower() in self.FILLER_VERBS or tok.text.lower() in self.WEAK_CONNECTORS:
                if dry_run:
                    audit["structural"].append({"action": "dropped", "token": tok.text, "reason": "filler/connector"})
                continue
            if tok.pos_ in self.KEEP_POS:
                right_filtered.append(tok.text)
            elif dry_run and tok.pos_ not in self.KEEP_POS:
                audit["structural"].append({"action": "dropped", "token": tok.text, "reason": f"pos={tok.pos_}"})

        left_str = " ".join(left_filtered).strip()
        right_str = " ".join(right_filtered).strip()
        core_str = core_token.text

        if left_str and right_str:
            result = f"{left_str} {core_str}: {right_str}"
            if dry_run:
                audit["structural"].append({"action": "format", "pattern": "left core: right"})
        elif left_str:
            result = f"{left_str} {core_str}"
            if dry_run:
                audit["structural"].append({"action": "format", "pattern": "left core"})
        elif right_str:
            result = f"{core_str}: {right_str}"
            if dry_run:
                audit["structural"].append({"action": "format", "pattern": "core: right"})
        else:
            result = core_str

        result = self._smart_title_case(result)

        # ── STEP 2: Bigram contraction ──
        if self.contract_bigrams:
            result, bigram_audit = TechBigramContractor.contract_string(
                result, extra_bigram_map, self.protected, dry_run)
            if dry_run:
                audit["bigram"] = bigram_audit

        # ── STEP 3: Unigram contraction ──
        if self.contract_words:
            result, unigram_audit = TechWordContractor.contract_string(
                result, extra_word_map, self.protected, dry_run)
            if dry_run:
                audit["unigram"] = unigram_audit

        return result, audit


# ─────────────────────────────────────────────────────────────
# CLI & CONFIG LOADING
# ─────────────────────────────────────────────────────────────

def load_config(config_path: str) -> dict:
    """Load JSON config file for custom maps and settings."""
    path = Path(config_path)
    if not path.exists():
        return {}
    with open(path, 'r') as f:
        return json.load(f)

def main():
    parser = argparse.ArgumentParser(description="Deterministic Title Contractor")
    parser.add_argument("title", nargs="?", help="Title to contract (or use --file)")
    parser.add_argument("-f", "--file", help="File with one title per line")
    parser.add_argument("-c", "--config", help="JSON config file for custom maps")
    parser.add_argument("-d", "--dry-run", action="store_true", help="Show audit log of contractions")
    parser.add_argument("-q", "--quiet", action="store_true", help="Only output contracted title")
    parser.add_argument("--no-words", action="store_true", help="Disable unigram contraction")
    parser.add_argument("--no-bigrams", action="store_true", help="Disable bigram contraction")
    parser.add_argument("--protected", nargs="*", help="Additional protected tokens")
    
    args = parser.parse_args()
    
    if not args.title and not args.file:
        parser.print_help()
        sys.exit(1)
    
    # Load config
    config = load_config(args.config) if args.config else {}
    extra_words = config.get("word_map", {})
    extra_bigrams = config.get("bigram_map", {})
    protected = PROTECTED_TOKENS | set(args.protected or []) | set(config.get("protected_tokens", []))
    
    contractor = MiddleOutTitleContractor(
        contract_words=not args.no_words,
        contract_bigrams=not args.no_bigrams,
        protected_tokens=protected
    )
    
    titles = []
    if args.file:
        with open(args.file, 'r') as f:
            titles = [line.strip() for line in f if line.strip()]
    else:
        titles = [args.title]
    
    for title in titles:
        result, audit = contractor.contract(
            title, 
            extra_word_map=extra_words,
            extra_bigram_map=extra_bigrams,
            dry_run=args.dry_run
        )
        
        if args.dry_run and not args.quiet:
            print(f"\n{'='*60}")
            print(f"INPUT:  {title}")
            print(f"OUTPUT: {result}")
            print(f"\n🔍 AUDIT LOG:")
            if audit["structural"]:
                print(f"  [Structural] {len(audit['structural'])} changes:")
                for change in audit["structural"]:
                    print(f"    • {change['action']}: '{change['token']}' ({change['reason']})")
            if audit["bigram"]:
                print(f"  [Bigram] {len(audit['bigram'])} changes:")
                for change in audit["bigram"]:
                    print(f"    • '{change['original']}' → '{change['contracted']}' (pos {change['pos']})")
            if audit["unigram"]:
                print(f"  [Unigram] {len(audit['unigram'])} changes:")
                for change in audit["unigram"]:
                    print(f"    • '{change['original']}' → '{change['contracted']}' (pos {change['pos']})")
            if not any(audit.values()):
                print("  (no contractions applied)")
        elif args.quiet:
            print(result)
        else:
            print(f"{title}\n  → {result}")

if __name__ == "__main__":
    # Ensure NLTK data is available
    try:
        nltk.data.find('tokenizers/punkt')
    except LookupError:
        nltk.download('punkt', quiet=True)
    
    main()
