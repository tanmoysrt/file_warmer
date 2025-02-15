### fwup - File Warm Up

This tool will do nothing but try to read file blocks as fast as possible.

Internally it will spawn up 50 lightweight threads (called goroutine) with 1MB of block size to read multiple part of file at a time.

#### Usage

```bash
fwup <file>
```

#### Why ?

When you create a volume from snapshot and attach to VM, the volume will be created instantly.

At that moment, the volume is not yet fully initialized. It will load only the blocks from S3 that are required to be loaded by the VM.

So, you need to read the file blocks, so that you can force underlying infra to load the volume.

AWS suggests to use `dd` or `fio` to read the file blocks.

```bash
sudo dd if=/dev/xvdf of=/dev/null bs=1M status=progress
```

> P.S : You can pass the file also instead of a volume.

**Issue With `dd` or `fio`**

`dd` and `fio` both built for different purpose. They are not designed to parallely read file blocks.

They are designed to read file sequentially.

This `fwup` is designed to get the disk blocks info of a file and read them parallely.


#### Best Suit Configuration

AWS has some kind of limitation that we can't cross 300 MB/s anyway while initialization.

**3000 IOPS 300MB/s**

```
fwup                                                                                                                                                        100% 2356KB 793.4KB/s   00:02    
Workers running: 5
Block size: 128 KB
File size: 10136477638 bytes
Number of blocks: 77336
Current throughput: 298.16 MB/s (Blocks processed: 11928)
Current throughput: 248.51 MB/s (Blocks processed: 21868)
Current throughput: 248.57 MB/s (Blocks processed: 31811)
Current throughput: 248.56 MB/s (Blocks processed: 41752)
Current throughput: 248.50 MB/s (Blocks processed: 51693)
Current throughput: 248.05 MB/s (Blocks processed: 61615)
Current throughput: 248.54 MB/s (Blocks processed: 71558)

~~~ Overall Stats ~~~ 
Total time: 37.91 seconds
Total data: 9667.00 MB
Average throughput: 255.02 MB/s
```

No overhead on running system.
Just the file read without storing in disk cache.

#### Some Tests

PS: This can be inaccurate based on the file. Need to play with the number of threads also to see what's the limitation by the underlying infra.

**Note :** 3000 iops + 250mb/s throughput

#### Without any intialization

```
ubuntu@ip-172-31-13-105:~$ time cp /mnt/app2/files-5GB /app/c8

real    6m31.665s
user    0m0.026s
sys     0m4.484s

```

Total time > 6m 35s

#### DD

```
ubuntu@ip-172-31-13-105:~$ dd if=/mnt/app2/files-5GB of=/dev/null bs=1M status=progress iflag=direct

4829741056 bytes (4.8 GB, 4.5 GiB) copied, 235 s, 20.6 MB/s
4768+1 records in
4768+1 records out
5000000000 bytes (5.0 GB, 4.7 GiB) copied, 235.654 s, 21.2 MB/s
```

```
ubuntu@ip-172-31-13-105:~$ time cp /mnt/app2/files-5GB /app/c11

real	0m37.172s
user	0m0.017s
sys	0m4.385s
```

Total time > 235+37 -> ~4.5m

#### fwup

Init with `fwup` and then do cp

```
ubuntu@ip-172-31-13-105:~$ /app/fwup /mnt/app2/files-5GB
File size: 5000000000 bytes
Number of blocks: 4769
Current throughput: 114.53 MB/s (Blocks processed: 573)
Current throughput: 181.90 MB/s (Blocks processed: 1482)
Current throughput: 182.45 MB/s (Blocks processed: 2395)
Current throughput: 182.94 MB/s (Blocks processed: 3309)
Current throughput: 181.00 MB/s (Blocks processed: 4214)

Initialization complete
Total time: 27.79 seconds
Total data processed: 4769.00 MB
Average throughput: 171.58 MB/s
```

```
ubuntu@ip-172-31-13-105:~$ time cp /mnt/app2/files-5GB /app/c8

real	0m37.144s
user	0m0.032s
sys	0m4.340s
```

Total time > 28+40 -> 1m

#### Fio

```
fio --filename=/mnt/app2/files-5GB --rw=read --bs=1M --iodepth=6 --ioengine=libaio --direct=1 --name=volume-initialize

Jobs: 1 (f=1): [R(1)][100.0%][r=40.0MiB/s][r=40 IOPS][eta 00m:00s]

~60s
```

```
ubuntu@ip-172-31-13-105:~$ time cp /mnt/app2/files-5GB /app/c11

real	0m37.172s
user	0m0.017s
sys	0m4.385s
```

Total time -> 100s -> 1.5m

