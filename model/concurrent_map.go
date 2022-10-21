package model

import (
	"sync"

	"github.com/golang/glog"

	"linearmodel/base"
	"linearmodel/optim"
)

type submap struct {
	mutex sync.Mutex
	data  map[uint64]*base.Parameter
}

type concurrentMap struct {
	modelData [concurrentCount]*submap
	bias_lock sync.Mutex
	bias      *base.Parameter
	size      uint32
	norm      float32
	eval      bool
}

func NewConcurrentMap(cap uint64, size uint32) *concurrentMap {
	cMap := &concurrentMap{}
	cMap.init(cap, size)
	return cMap
}

func (b *concurrentMap) init(cap uint64, size uint32) error {
	b.bias = &base.Parameter{}
	for i := 0; i < concurrentCount; i++ {
		b.modelData[i] = &submap{data: make(map[uint64]*base.Parameter, cap/concurrentCount), mutex: sync.Mutex{}}
	}
	b.size = size
	b.eval = false
	b.norm = float32(b.size)
	return nil
}

func (b *concurrentMap) lock(key uint64) {
	b.modelData[key%concurrentCount].mutex.Lock()
}

func (b *concurrentMap) unlock(key uint64) {
	b.modelData[key%concurrentCount].mutex.Unlock()
}

func (b *concurrentMap) update(key uint64, slot uint16, label int, grad float32, opt optim.Optimizer) {
	//key uint64, slot uint16
	if key == 0 {
		opt.Update(grad, b.bias)
		return
	}
	b.lock(key)
	p, ok := b.modelData[key%concurrentCount].data[key]
	b.unlock(key)
	if !ok {
		glog.Errorf(">>>> update parameter before exist: key=%d, slot=%d", key, slot)
		return
	}
	p.Show += 1
	p.Click += label
	opt.Update(grad, p)
	return
}

func (b *concurrentMap) updateEmb(key uint64, slot uint16, label int, grad []float32, opt optim.Optimizer) {
	b.lock(key)
	p, ok := b.modelData[key%concurrentCount].data[key]
	b.unlock(key)
	if !ok || len(p.VecW) != int(b.size) || len(p.VecN) != int(b.size) || len(p.VecZ) != int(b.size) {
		glog.Errorf(">>>> update parameter before exist: key=%d, slot=%d, w=%d, n=%d, z=%d", key, slot,
			len(p.VecW), len(p.VecN), len(p.VecZ))
		return
	}
	p.Show += 1
	p.Click += label
	opt.UpdateEmb(grad, p)
}

func (b *concurrentMap) getWeight(key uint64, slot uint16, needInit bool) *base.Weight {
	if key == 0 {
		p := b.bias
		return &base.Weight{W: p.W}
	}
	w := base.Weight{W: 0.0, VecW: make([]float32, b.size)}
	b.lock(key)
	p := b.modelData[key%concurrentCount].data[key]
	if p == nil && needInit {
		p = base.NewParameter(b.size)
		p.Slot = slot
		p.Fea = key
		b.modelData[key%concurrentCount].data[key] = p
	}
	b.unlock(key)
	if p != nil {
		w.W = p.W
		copy(w.VecW, p.VecW)
	}
	return &w
}

func (b *concurrentMap) get(key uint64, slot uint16, needInit bool) *base.Parameter {
	if key == 0 {
		b.bias_lock.Lock()
		p := b.bias
		b.bias_lock.Unlock()
		return p
	}
	b.lock(key)
	p := b.modelData[key%concurrentCount].data[key]
	if p == nil && needInit {
		p = base.NewParameter(b.size)
		p.Slot = slot
		p.Fea = key
		b.modelData[key%concurrentCount].data[key] = p
	}
	b.unlock(key)
	return p
}

func (b *concurrentMap) set(key uint64, parameter *base.Parameter) {
	if key == 0 {
		b.bias_lock.Lock()
		b.bias = parameter
		b.bias_lock.Unlock()
		return
	}
	b.lock(key)
	b.modelData[key%concurrentCount].data[key] = parameter
	b.unlock(key)
}

//func (b *concurrentMap) save(p string) error {
//	f, err := os.Create(p)
//	defer f.Close()
//
//	if err != nil {
//		glog.Errorf("creat path: %s fail", p)
//		return err
//	}
//	fmt.Fprintf(f, "0\t%.8f\n", b.bias.W)
//	for _, x := range b.modelData {
//		for k, v := range x.data {
//			fmt.Fprintf(f, "%d\t%.8f\n", k, v.W)
//		}
//	}
//	return nil
//}
//
//func (b *concurrentMap) load(p string) error {
//	f, err := os.Open(p)
//	defer f.Close()
//	if err != nil {
//		glog.Errorf("load path error: %s", err)
//		return err
//	}
//	r := bufio.NewReader(f)
//	count := 0
//	for {
//		bt, err := r.ReadBytes('\n')
//		// fmt.Println(string(bt), err)
//		count++
//		if err != nil {
//			break
//		}
//		line := strings.TrimSuffix(string(bt), "\n")
//		row := strings.Split(line, "\t")
//		if len(row) != 2 {
//			glog.Fatalf("wrong line[%d]: %s in file %s", count, line, p)
//		}
//		key, err := strconv.ParseUint(row[0], 10, 64)
//		if err != nil {
//			glog.Errorf("wrong key[%d]: %s in file %s, err=%s", count, row[0], p, err)
//			continue
//		}
//		val, err := strconv.ParseFloat(row[1], 10)
//		if err != nil {
//			glog.Errorf("wrong key[%d]: %s in file %s, err=%s", count, row[1], p, err)
//			continue
//		}
//		pm := new(base.Parameter)
//		pm.W = val
//		b.set(key, pm)
//	}
//	return nil
//}
//
//func (b *concurrentMap) save_inc(p string) error {
//	f, err := os.Create(p)
//	defer f.Close()
//
//	if err != nil {
//		glog.Errorf("creat path: %s fail", p)
//		return err
//	}
//	fmt.Fprintf(f, "0\t%.8f\t%.8f\t%.8f\n", b.bias.W, b.bias.Z, b.bias.N)
//	for _, x := range b.modelData {
//		for k, v := range x.data {
//			fmt.Fprintf(f, "%d\t%.8f\t%.8f\t%.8f\n", k, v.W, v.Z, v.N)
//		}
//	}
//	return nil
//}
//
//func (b *concurrentMap) load_inc(p string) error {
//	f, err := os.Open(p)
//	defer f.Close()
//	if err != nil {
//		glog.Errorf("load path error: %s", err)
//		return err
//	}
//	r := bufio.NewReader(f)
//	count := 0
//	for {
//		bt, err := r.ReadBytes('\n')
//		// fmt.Println(string(bt), err)
//		count++
//		if err != nil {
//			break
//		}
//		line := strings.TrimSuffix(string(bt), "\n")
//		row := strings.Split(line, "\t")
//		if len(row) != 4 {
//			glog.Fatalf("wrong line[%d]: %s in file %s", count, line, p)
//		}
//		key, err := strconv.ParseUint(row[0], 10, 64)
//		if err != nil {
//			glog.Errorf("wrong key[%d]: %s in file %s, err=%s", count, row[0], p, err)
//			continue
//		}
//		val_w, err := strconv.ParseFloat(row[1], 10)
//		if err != nil {
//			glog.Errorf("wrong key[%d]: %s in file %s, err=%s", count, row[1], p, err)
//			continue
//		}
//		val_z, err := strconv.ParseFloat(row[2], 10)
//		if err != nil {
//			glog.Errorf("wrong key[%d]: %s in file %s, err=%s", count, row[2], p, err)
//			continue
//		}
//		val_n, err := strconv.ParseFloat(row[3], 10)
//		if err != nil {
//			glog.Errorf("wrong key[%d]: %s in file %s, err=%s", count, row[3], p, err)
//			continue
//		}
//		pm := new(base.Parameter)
//		pm.W = val_w
//		pm.Z = val_z
//		pm.N = val_n
//		b.set(key, pm)
//	}
//	return nil
//}