package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.etcd.io/etcd/clientv3"
	"go.etcd.io/etcd/clientv3/concurrency"
	recipe "go.etcd.io/etcd/contrib/recipes"
)

// https://pkg.go.dev/github.com/keob/etcd/clientv3
// https://github.com/helios741/myblog/tree/new/learn_go/src/2020/0308_etcd_go_client

func TestClient(t *testing.T) {
	// 客户端配置
	config := clientv3.Config{
		Endpoints:   []string{"127.0.0.1:2379"},
		DialTimeout: 5 * time.Second,
	}
	// 建立连接
	client, err := clientv3.New(config)
	require.NoError(t, err)

	{
		kv := clientv3.NewKV(client)

		// Put
		_, err = kv.Put(context.TODO(), "/test", "hello etcd")
		require.NoError(t, err)

		// Get
		_, err = kv.Get(context.TODO(), "/test")
		require.NoError(t, err)

		// Delete
		_, err = kv.Delete(context.TODO(), "/test", clientv3.WithPrevKV())
		require.NoError(t, err)
	}

	{ // 申请租约，定时查看是否过期
		lease := clientv3.NewLease(client)
		leaseGrantResp, err := lease.Grant(context.TODO(), 5)
		require.NoError(t, err)

		kv := clientv3.NewKV(client)

		_, err = kv.Put(context.TODO(), "/leaseTick", "hello etcd", clientv3.WithLease(leaseGrantResp.ID))
		require.NoError(t, err)

		for {
			getResp, err := kv.Get(context.TODO(), "/leaseTick")
			require.NoError(t, err)

			if getResp.Count == 0 {
				fmt.Println("kv过期了")
				break
			}
			fmt.Println("还没过期:", getResp.Kvs)
			time.Sleep(2 * time.Second)
		}
	}

	{ // 自动续租
		lease := clientv3.NewLease(client)
		leaseGrantResp, err := lease.Grant(context.TODO(), 5)
		require.NoError(t, err)

		keepRespChan, err := lease.KeepAlive(context.TODO(), leaseGrantResp.ID)
		require.NoError(t, err)

		kv := clientv3.NewKV(client)

		_, err = kv.Put(context.TODO(), "/leaseAuto", "hello etcd", clientv3.WithLease(leaseGrantResp.ID))
		require.NoError(t, err)

		go func() {
			for keepResp := range keepRespChan {
				if keepRespChan == nil {
					fmt.Println("租约已经失效了")
					return
				}

				// 每秒会续租一次, 所以就会受到一次应答
				fmt.Println("收到自动续租应答:", keepResp.ID)
			}
		}()
	}

	{ // watch功能
		kv := clientv3.NewKV(client)

		// 模拟KV的变化
		go func() {
			for {
				_, err = kv.Put(context.TODO(), "/testWatch", "hello etcd")
				require.NoError(t, err)
				_, err = kv.Delete(context.TODO(), "/testWatch")
				require.NoError(t, err)
				time.Sleep(1 * time.Second)
			}
		}()

		// 先GET到当前的值，并监听后续变化
		getResp, err := kv.Get(context.TODO(), "/testWatch")
		require.NoError(t, err)

		// 现在key是存在的
		if len(getResp.Kvs) != 0 {
			fmt.Println("当前值:", string(getResp.Kvs[0].Value))
		}

		// 获得当前revision
		watchStartRevision := getResp.Header.Revision + 1
		// 创建一个watcher
		watcher := clientv3.NewWatcher(client)
		fmt.Println("从该版本向后监听:", watchStartRevision)

		ctx, cancelFunc := context.WithCancel(context.TODO())
		time.AfterFunc(5*time.Second, func() {
			cancelFunc()
		})

		watchRespChan := watcher.Watch(ctx, "/testWatch", clientv3.WithRev(watchStartRevision))
		// 处理kv变化事件
		for watchResp := range watchRespChan {
			for _, event := range watchResp.Events {
				switch event.Type {
				case 0:
					fmt.Println("修改为:", string(event.Kv.Value), "Revision:", event.Kv.CreateRevision, event.Kv.ModRevision)
				case 1:
					fmt.Println("删除了", "Revision:", event.Kv.ModRevision)
				}
			}
		}
	}

	{ // 通过txn实现分布式锁
		// 1. 上锁
		// 1.1 创建租约
		lease := clientv3.NewLease(client)

		leaseGrantResp, err := lease.Grant(context.TODO(), 5)
		require.NoError(t, err)

		// 1.2 自动续约
		// 创建一个可取消的租约，主要是为了退出的时候能够释放
		ctx, cancelFunc := context.WithCancel(context.TODO())

		// 3. 释放租约
		defer cancelFunc()
		defer lease.Revoke(context.TODO(), leaseGrantResp.ID)

		keepRespChan, err := lease.KeepAlive(ctx, leaseGrantResp.ID)
		require.NoError(t, err)

		// 续约应答
		go func() {
			for keepResp := range keepRespChan {
				if keepRespChan == nil {
					fmt.Println("租约已经失效了")
					return
				}

				// 每秒会续租一次, 所以就会受到一次应答
				fmt.Println("收到自动续租应答:", keepResp.ID)
			}
		}()

		// 1.3 在租约时间内去抢锁（etcd里面的锁就是一个key）
		kv := clientv3.NewKV(client)

		// 创建事物
		txn := kv.Txn(context.TODO())

		//if 不存在key， then 设置它, else 抢锁失败
		txn.If(clientv3.Compare(clientv3.CreateRevision("/testTxn"), "=", 0)).
			Then(clientv3.OpPut("/testTxn", "hello etcd", clientv3.WithLease(leaseGrantResp.ID))).
			Else(clientv3.OpGet("/testTxn"))

		// 提交事务
		txnResp, err := txn.Commit()
		require.NoError(t, err)

		if !txnResp.Succeeded {
			fmt.Println("锁被占用:", string(txnResp.Responses[0].GetResponseRange().Kvs[0].Value))
			return
		}

		// 2. 抢到锁后执行业务逻辑，没有抢到退出
		fmt.Println("处理任务")
		time.Sleep(5 * time.Second)

		// 3. 释放锁，步骤在上面的defer，当defer租约关掉的时候，对应的key被回收了
	}

	select {}
}

////////////////////////////////////////////////////////////

// https://blog.csdn.net/cyq6239075/article/details/109862443
// https://cloud.tencent.com/developer/article/1458456

const prefix = "/election-demo"
const prop = "local"

var leaderFlag bool

func TestMaster(t *testing.T) {
	endpoints := []string{"127.0.0.1:2379"}
	donec := make(chan struct{})

	cli, err := clientv3.New(clientv3.Config{Endpoints: endpoints})
	if err != nil {
		log.Fatal(err)
	}
	defer cli.Close()

	go campaign(cli, prefix, prop)

	go func() {
		for {
			<-time.After(5 * time.Second)
			doCrontab()
		}
	}()

	<-donec
}

func campaign(c *clientv3.Client, election string, prop string) {
	for {
		session, err := concurrency.NewSession(c, concurrency.WithTTL(15))
		if err != nil {
			fmt.Println(err)
			continue
		}
		e := concurrency.NewElection(session, election)
		ctx := context.TODO()

		if err = e.Campaign(ctx, prop); err != nil {
			fmt.Println(err)
			continue
		}

		fmt.Println("elect: success")
		leaderFlag = true

		select {
		case <-session.Done():
			leaderFlag = false
			fmt.Println("elect: expired")
		}
	}
}

func doCrontab() {
	if leaderFlag == true {
		fmt.Println("doCrontab")
	}
}

////////////////////////////////////////////////////////////

// http://qiuwenqi.com/2020-11-27-golang-concurrency-etcd-queue-barrier.html
// https://cloud.tencent.com/developer/article/1458456

var (
	addr = flag.String("addr", "http://127.0.0.1:2379", "etcd addresses")
)

func TestFF(t *testing.T) {
	flag.Parse()

	endpoints := strings.Split(*addr, ",")

	cli, err := clientv3.New(clientv3.Config{Endpoints: endpoints})

	if err != nil {
		log.Fatal(err)
	}
	defer cli.Close()
	// 创建/获取队列
	q := recipe.NewPriorityQueue(cli, "my-test-queue")

	go func() {
		q := recipe.NewPriorityQueue(cli, "my-test-queue")
		for {
			time.Sleep(time.Second)
			q.Enqueue("tewtewtwe", 1)
			fmt.Println("in")
		}
	}()

	for {
		v, err := q.Dequeue() //出队
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("out", v)
	}
}

// https://gist.github.com/thrawn01/c007e6a37b682d3899910e33243a3cdc
// https://cloud.tencent.com/developer/article/1458456
