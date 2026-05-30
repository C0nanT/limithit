# Security & Authorized Use

## Authorized use only

limithit sends real attack traffic — request floods, connection exhaustion, header
overflows, decompression bombs, and more. **You must have explicit written
authorisation from the system owner before running any attack.**

Unauthorized use may violate the Computer Fraud and Abuse Act (US), the Computer
Misuse Act (UK), and equivalent laws in your jurisdiction. The authors accept no
liability for misuse.

## Scope guardrails built into the tool

| Guardrail | Default | Override |
|---|---|---|
| Non-loopback warning | printed on stderr | `--allow-target <host>` |
| Amplifying attacks (gzipbomb) | blocked | `--i-understand` |
| Global RPS cap | uncapped | `--max-rps <n>` |
| Identifying User-Agent | `limithit/<version>` | `--header "User-Agent: ..."` |
| Audit log | off | `--audit-log <path>` |

## Vulnerability disclosure

If you discover a security issue in limithit itself, please open a GitHub issue
marked **[Security]** or email the maintainer directly. Do not disclose publicly
before a fix is available.
