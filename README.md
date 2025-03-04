### fwup - File Warm Up

This tool will do nothing but try to read file blocks as fast as possible.

It's useful to initialize/pre-warm volumes.

### Installation

```bash
pip install filewarmer
```

### Usage

```python
from filewarmer import FWUP

fwup = FWUP()
fwup.warmup(
    ["./25gb.glass", "./1gb.glass", "/var/lib/mysql/abc_demo/tab@020Sales@020Invoice.ibd", "/var/lib/mysql/abc_demo/tab@020Sales@020Invoice.ibd"]
    method="io_uring",
    small_file_size_threshold=1024 * 1024,
    block_size_for_small_files=256 * 1024,
    block_size_for_large_files=256 * 1024,
    small_files_worker_count=1,
    large_files_worker_count=1,
)
```

**Notes -**

- For io_uring, it's recommended to use Linux Kernel 5.1 or higher.
- For io_uring, use a single thread to submit the requests.

### Build + Publish

```bash
TWINE_PASSWORD=xxxxxx VERSION=0.0.10 ./build.sh
```

### License

Apache 2.0
