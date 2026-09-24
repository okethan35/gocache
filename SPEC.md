# Phase 1 spec — gocache store

Commands supported: SET key value [ttl_seconds], GET key, DEL key

1. GET on a missing key returns: (empty value, False) value plus "bad" flag, either nil or "" depending on value
2. GET on an expired key returns: (empty value, False) value plus "bad" flag, either nil or "" depending on value
3. Expiry checking strategy: lazy, get at GET time
4. SET with ttl_seconds = 0 means: key expires immediately — a GET issued 
   at the exact same instant as SET, with zero real time elapsed, is 
   already treated as a miss.
5. SET with ttl_seconds < -1 means: ttl_seconds get compressed to -1
6. SET with ttl_seconds = -1 (or not provided by client) means: explicitly not set, no expiration
7. SET on an existing key: does it fully replace the old value and TTL? yes
8. DEL on a missing key: error, or silently do nothing? return 0 (number of deleted keys)
9. SET or DEL on empty key: return 0 (number of keys updated)
10. SET on valid key: return 1


# Phase 2 spec

## 2a - Protocol + single connection, no concurrency

### Wire format
1. Commands follow the format: SET foo $bar$ 30\n (newline-terminated)
   values must be wrapped in dollar signs. Dollar signs are forbidden input.
2. commands are case sensitive. first word must be a fully capital command. everything else is case insensitive. 
3. Keys are one word only.
4. Values are wrapped in $.
5. GET and DEL will only take one argument, the variable. e.g. GET foo, DEL foo

### Responses
6. A successful SET response on the wire looks like: 1\n
7. A successful GET response on the wire looks like: True $bar$\n
8. A DEL response will return either 0 or 1
9. An unsuccessful SET response on the wire looks like: 0\n
10. An unsuccessful GET response on the wire looks like: False $nil$\n

### Parsing Algorithm
11. After command word, the next word is the key. 
12. For SET, next look for a $. anything contained between first and second $ is the argument. Everything after the closing $ is the ttl. If there is none, ttl is -1.
13. For GET and DEL, any other trailing input returns: invalid command\n.

### Malformed input
14. Command with no key: invalid command\n
15. SET with no value: invalid command\n
16. SET with non-integer ttls return: invalid command\n
17. Unknown command: invalid command\n
18. Empty line: invalid command\n
19. If more dollar signs are in input than allowed for command (0 for GET and DEL, 2 for SET), then stop and return: invalid command\n
20. On any malformed input, keep connection open and keep parsing for the next commands

### connection lifecycle
21. One connection can send multiple commands
22. If connection is interrupted mid command, discard input and close connection.
23. Only support line lengths of 256 characters. If it's exceeded, return "max line length exceeded\n". Keep connection open.

### Server startup
24. Server listens on port 6380.

### Misc
25. Empty values ($$) will be accepted. Empty values may be stored.