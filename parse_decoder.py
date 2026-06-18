import re, os, sys

path = os.path.join(os.path.dirname(sys.modules['ibapi'].__file__), 'orderdecoder.py')
with open(path) as f:
    code = f.read()

# We just want to write a Go struct or sequence of cursor.next() calls.
# This might be complex to parse statically because of `if version >= X`.
# Instead of doing that, I'll just write a Go generator that reads the fields.
