# ThreadMe
Run any cli command in concurrent mode. 
Choose how many threads you need (default: 5).

## Usage


### Simple
Run command with default params
```sh
./threadme -cmd "date +%s"
```
Output:
```
             Command: [date +%s]
Job count to perform: [6]
             Threads: [5]
               Delay: 10 ms
Time limit for single worker: 60000 ms
--------------------------------------------------------------------------------
2022/04/15 15:25:17 OUTPUT:[4/6] [date +%s] ==> [1650025517]
2022/04/15 15:25:17 OUTPUT:[2/6] [date +%s] ==> [1650025517]
2022/04/15 15:25:17 OUTPUT:[1/6] [date +%s] ==> [1650025517]
2022/04/15 15:25:17 OUTPUT:[3/6] [date +%s] ==> [1650025517]
...
```

## Dynamic number input #1
Use param `{{N}}` to use job number.

```sh
./threadme -cmd 'wc -l {{N}}.txt' 
```
Output:
```
2022/04/15 15:39:24 [4/10] [wc -l 4.txt] ==> [3 4.txt]
2022/04/15 15:39:24 ERROR: [2/10] [wc -l 2.txt] ==> [wc: 2.txt: No such file or directory; exit status 1;]
2022/04/15 15:39:24 ERROR: [6/10] [wc -l 6.txt] ==> [wc: 6.txt: No such file or directory; exit status 1;]
2022/04/15 15:39:24 [7/10] [wc -l 7.txt] ==> [1 7.txt]
2022/04/15 15:39:24 ERROR: [8/10] [wc -l 8.txt] ==> [wc: 8.txt: No such file or directory; exit status 1;]
...
```


## Read command input from file 
Use param `{{LINE}}` to use job param from file.

```sh
./threadme -cmd 'bash sendmail.sh {{LINE}}' -f customers.txt
```
Output:
```
2022/04/15 15:52:36 [0/8256] [bash sendmail.sh a@a.example.com] ==> [Sent: a@a.example.com]
2022/04/15 15:39:24 ERROR: [4/8256] [bash sendmail.sh e@e.example.com] ==> [Invalid user; exit status 1;]
2022/04/15 15:52:36 [2/8256] [bash sendmail.sh c@c.example.com] ==> [Sent: c@c.example.com]
2022/04/15 15:52:36 [1/8256] [bash sendmail.sh b@b.example.com] ==> [Sent: b@b.example.com]
2022/04/15 15:52:36 [3/8256] [bash sendmail.sh d@d.example.com] ==> [Sent: d@d.example.com]
...
```

## Stop all jobs if error message occurs 
Use `-stop-on` flag to halt all tasks upon encountering an error message.
Note: All tasks will be discontinued if any job yields a message indicating a stop condition.

```sh
./threadme -cmd 'bash sendmail.sh {{LINE}}' -f customers.txt -stop-on 'Error:'
```
Output:
```
2023/11/10 15:52:36 [0/8256] [bash sendmail.sh a@a.example.com] ==> [Sent: a@a.example.com]
2023/11/10 15:39:24 ERROR: [4/8256] [bash sendmail.sh e@e.example.com] ==> [Invalid user; exit status 1;]
2023/11/10 15:52:36 [2/8256] [bash sendmail.sh c@c.example.com] ==> [Sent: c@c.example.com]
2023/11/10 15:52:36 [1/8256] [bash sendmail.sh b@b.example.com] ==> [Sent: b@b.example.com]
2023/11/10 15:52:36 [3/8256] [bash sendmail.sh d@d.example.com] ==> [Sent: d@d.example.com]
2023/11/10 15:52:37 [4/8256] [bash sendmail.sh e@e.example.com] ==> [Error: timeout while sending e@e.example.com]
2023/11/10 15:03:57 > Stop output message found: Error: timeout while sending e@e.example.com
2023/11/10 15:52:37 > Stopping all workers!
2023/11/10 15:52:37 > Duration: 12.03473712s
exit status 1
```

## Keep running while message present 
Use `-while` flag to cease all tasks when a success message is absent from any of the jobs.
_Note_: All jobs will be terminated if the message differs for even a single job.

```sh
./threadme -cmd 'bash sendmail.sh {{LINE}}' -f customers.txt -while 'Sent:'
```
Output:
```
2023/11/10 15:52:36 [0/8256] [bash sendmail.sh a@a.example.com] ==> [Sent: a@a.example.com]
2023/11/10 15:39:24 ERROR: [4/8256] [bash sendmail.sh e@e.example.com] ==> [Invalid user; exit status 1;]
2023/11/10 15:52:37 > No `while message` found: Invalid user; exit status 1;
2023/11/10 15:52:37 > Stopping all workers!
2023/11/10 15:52:37 > Duration: 7.03404163s 
exit status 1
```


## Use different interpreter 
Use `-interpreter` flag to change command interpreter. By default it's `/bin/bash`.
_Default_: `/bin/bash`

#### Have `$RANDOM` value
```sh
# default is `/bin/bash`
./threadme -cmd 'echo "$USER $RANDOM"' -n=1
# 2023/11/10 18:47:57 [0/1] [echo $USER $RANDOM] ==> [bobiverse 31459]
```

#### Don't have `$RANDOM` value
```sh
./threadme -cmd 'echo "$USER $RANDOM"' -n=1 -interpreter=/bin/sh
# 2023/11/10 18:47:57 [0/1] [echo $USER $RANDOM] ==> [bobiverse]
```


