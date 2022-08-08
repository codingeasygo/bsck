package dialer

import (
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"time"

	"github.com/codingeasygo/util/converter"
	"github.com/codingeasygo/util/xmap"
	"github.com/codingeasygo/util/xtime"
)

type MapIntSorter struct {
	List  []string
	Data  map[string][]int64
	Index int
}

func NewMapIntSorter(data map[string][]int64, index int) *MapIntSorter {
	sorter := &MapIntSorter{
		Data:  data,
		Index: index,
	}
	for name := range data {
		sorter.List = append(sorter.List, name)
	}
	return sorter
}

func (m *MapIntSorter) Len() int {
	return len(m.List)
}

func (m *MapIntSorter) Less(i, j int) bool {
	return m.Data[m.List[i]][m.Index] < m.Data[m.List[j]][m.Index]
}

func (m *MapIntSorter) Swap(i, j int) {
	m.List[i], m.List[j] = m.List[j], m.List[i]
}

type BalancedPolicy struct {
	Matcher *regexp.Regexp
	Scope   string
	Limit   []int64
}

type BalancedFilter struct {
	Matcher *regexp.Regexp
	Access  int
}

type BalancedDialer struct {
	ID              string
	dialers         map[string]Dialer
	dialersUsed     map[string][]int64            //map key to [begin,used,fail]
	dialersHostUsed map[string]map[string][]int64 //map key/host to [begin,used,fail]
	dialersLock     chan int
	PolicyList      []*BalancedPolicy
	Filters         []*BalancedFilter
	Delay           int64
	Timeout         int64
	Conf            xmap.M
	matcher         *regexp.Regexp
}

func NewBalancedDialer() *BalancedDialer {
	dialer := &BalancedDialer{
		dialers:         map[string]Dialer{},
		dialersUsed:     map[string][]int64{},
		dialersHostUsed: map[string]map[string][]int64{},
		dialersLock:     make(chan int, 1),
		Delay:           500,
		Timeout:         3000,
		Conf:            xmap.M{},
		matcher:         regexp.MustCompile(".*"),
	}
	dialer.dialersLock <- 1
	return dialer
}

func (b *BalancedDialer) sortedDialer(index int) []string {
	sorter := NewMapIntSorter(b.dialersUsed, index)
	sort.Sort(sorter)
	return sorter.List
}

func (b *BalancedDialer) AddPolicy(matcher string, limit []int64) (err error) {
	if len(limit) < 2 {
		err = fmt.Errorf("limit must be [time,limit]")
		return
	}
	reg, err := regexp.Compile(matcher)
	if err == nil {
		b.PolicyList = append(b.PolicyList, &BalancedPolicy{
			Matcher: reg,
			Limit:   limit,
		})
	}
	return
}

func (b *BalancedDialer) AddFilter(matcher string, access int) (err error) {
	reg, err := regexp.Compile(matcher)
	if err == nil {
		b.Filters = append(b.Filters, &BalancedFilter{
			Matcher: reg,
			Access:  access,
		})
	}
	return
}

func (b *BalancedDialer) AddDialer(dialers ...Dialer) {
	<-b.dialersLock
	for _, dialer := range dialers {
		name := dialer.Name()
		b.dialers[name] = dialer
		b.dialersUsed[name] = []int64{0, 0, 0}
		b.dialersHostUsed[name] = map[string][]int64{}
	}
	b.dialersLock <- 1
}

func (b *BalancedDialer) Name() string {
	return b.ID
}

//initial dialer
func (b *BalancedDialer) Bootstrap(options xmap.M) (err error) {
	b.Conf = options
	b.ID = options.Str("id")
	if len(b.ID) < 1 {
		err = fmt.Errorf("the dialer id is required")
		return
	}
	matcher := options.Str("matcher")
	if len(matcher) > 0 {
		b.matcher, err = regexp.Compile(matcher)
	}
	b.Timeout = options.Int64Def(3000, "timeout")
	b.Delay = options.Int64Def(500, "delay")
	policy := options.ArrayMapDef(nil, "policy")
	for _, p := range policy {
		err = b.AddPolicy(p.Str("matcher"), p.ArrayInt64Def([]int64{}, "limit"))
		if err != nil {
			return
		}
	}
	filter := options.ArrayMapDef(nil, "filter")
	for _, f := range filter {
		err = b.AddFilter(f.Str("matcher"), int(f.Int64("access")))
		if err != nil {
			return
		}
	}
	<-b.dialersLock
	defer func() {
		b.dialersLock <- 1
	}()
	dialerOptions := options.ArrayMapDef(nil, "dialers")
	for _, option := range dialerOptions {
		dtype := option.Str("type")
		dialer := NewDialer(dtype)
		if dialer == nil {
			return fmt.Errorf("create dialer fail with type(%v) not supported by %v", dtype, converter.JSON(option))
		}
		err := dialer.Bootstrap(option)
		if err != nil {
			return err
		}
		name := dialer.Name()
		b.dialers[name] = dialer
		b.dialersUsed[name] = []int64{0, 0, 0}
		b.dialersHostUsed[name] = map[string][]int64{}
		DebugLog("BalancedDialer add dialer(%v) to pool success", dialer)
	}
	return nil
}

//Options
func (b *BalancedDialer) Options() xmap.M {
	return b.Conf
}

//Matched uri
func (b *BalancedDialer) Matched(uri string) bool {
	return b.matcher.MatchString(uri)
}

func (b *BalancedDialer) Dial(channel Channel, sid uint64, uri string, pipe io.ReadWriteCloser) (r Conn, err error) {
	for _, f := range b.Filters {
		if f.Matcher.MatchString(uri) {
			if f.Access < 1 {
				err = fmt.Errorf("access deny")
				return
			}
			break
		}
	}
	target, err := url.Parse(uri)
	if err != nil {
		return
	}
	//
	begin := xtime.Now()
	var showed int64
	failed := map[string]int{}
	for {
		now := xtime.Now()
		if now-begin >= b.Timeout {
			err = fmt.Errorf("dial to %v timeout", uri)
			break
		}
		<-b.dialersLock
		//do dialer limit
		sortedNames := b.sortedDialer(1)
		var limitedNames []string
		now = xtime.Now()
		for _, name := range sortedNames {
			if failed[name] > 2 {
				continue
			}
			dialer := b.dialers[name]
			used := b.dialersUsed[name]
			limit := dialer.Options().ArrayInt64Def(nil, "limit")
			if len(limit) < 2 {
				limitedNames = append(limitedNames, name)
				used[1] = 0
				continue
			}
			if now-used[0] > limit[0] {
				limitedNames = append(limitedNames, name)
				used[1] = 0
			}
			if used[1] < limit[1] {
				limitedNames = append(limitedNames, name)
			}
		}
		//do host policy
		var policy *BalancedPolicy
		for _, p := range b.PolicyList {
			if p.Matcher.MatchString(uri) {
				policy = p
				break
			}
		}
		var policyNames []string
		if policy == nil {
			policyNames = limitedNames
		} else {
			for _, name := range limitedNames {
				allHostUsed := b.dialersHostUsed[name]
				used := allHostUsed[target.Host]
				if used == nil {
					used = []int64{0, 0, 0}
					allHostUsed[target.Host] = used
				}
				if now-used[0] > policy.Limit[0] {
					policyNames = append(policyNames, name)
					used[1] = 0
				}
				if used[1] < policy.Limit[1] {
					policyNames = append(policyNames, name)
				}
			}
		}
		for _, name := range policyNames {
			dialer := b.dialers[name]
			if !dialer.Matched(uri) {
				continue
			}
			used := b.dialersUsed[name]
			hostUsed := b.dialersHostUsed[name][target.Host]
			if hostUsed == nil {
				hostUsed = []int64{0, 0, 0}
				b.dialersHostUsed[name][target.Host] = hostUsed
			}
			if used[1] == 0 {
				used[0] = xtime.Now()
			}
			if hostUsed[1] == 0 {
				hostUsed[0] = xtime.Now()
			}
			used[1]++
			hostUsed[1]++
			b.dialersLock <- 1
			r, err = dialer.Dial(channel, sid, uri, pipe)
			<-b.dialersLock
			if err == nil {
				used[2] = 0
				hostUsed[2] = 0
				b.dialersLock <- 1
				DebugLog("BalancedDialer dail to %v with dialer(%v) success", uri, dialer)
				return
			}
			failed[name]++
			used[2]++
			hostUsed[2]++
			DebugLog("BalancedDialer using %v and dial to %v fail with %v", dialer, uri, err)
			failRemove := dialer.Options().Int64Def(0, "fail_remove")
			if failRemove > 0 && used[2] >= failRemove {
				DebugLog("BalancedDialer remove dialer(%v) by %v fail count", dialer, used[2])
				delete(b.dialers, name)
				delete(b.dialersUsed, name)
				delete(b.dialersHostUsed, name)
			}
		}
		b.dialersLock <- 1
		now = xtime.Now()
		if now-showed > 3000 {
			DebugLog("BalancedDialer dial to %v is waiting to connect", uri)
			showed = now
		}
		time.Sleep(time.Duration(b.Delay) * time.Millisecond)
	}
	return
}

//Shutdown will shutdown dial
func (b *BalancedDialer) Shutdown() (err error) {
	return
}
