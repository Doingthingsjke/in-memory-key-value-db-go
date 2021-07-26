package server

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"
	// "unsafe"
)

const (
	maxSize           int           = 50
	maxUsers          int           = 5
	defaultExpiration time.Duration = 0
	noExpiration      time.Duration = -1
)

type MemoryDB struct {
	*memoryDB
}

// Структура для БД
type memoryDB struct {
	// maxSize            int
	// maxUsers           int
	items              map[string]Item
	mu                 sync.RWMutex
	defaultExpiration  time.Duration
	garbageCollector   *garbageCollector
	onDeleted          func(string, interface{})
}

// Структура сборщика мусора
type garbageCollector struct {
	Interval time.Duration
	stop     chan bool
}

// Структура для одного элемента
type Item struct {
	Value      interface{}
	Created    time.Time
	Expiration int64
}

type keyAndValue struct {
	key   string
	value interface{}
}

// // Функция для проверки срока жизни элемента
// func (item *Item) isExpire() bool {
// 	// если 0, то значит срок жизни элемента бесконечный
// 	if item.Expiration == 0 {
// 		return false
// 	}

// 	return time.Now().UnixNano() > item.Expiration
// }
// Функция для инициализации новой in-memory key-value bd
func newDB(defaultExpiration, cleanupInterval time.Duration) *MemoryDB {
	items := make(map[string]Item)
	return NewMemoryDbWithGarbageCollector(defaultExpiration, cleanupInterval, items)
}
// Функция для инициализации новой in-memory key-value bd со значениями из db.json
func newDbFrom(defaultExpiration, cleanupInterval time.Duration) *MemoryDB{
	f, err := os.Open("db.json")
	if err != nil {
		return NewMemoryDbWithGarbageCollector(defaultExpiration, cleanupInterval, map[string]Item{})
	}

	items := make(map[string]Item)
	if err := json.NewDecoder(f).Decode(&items); err != nil {
		fmt.Println("Couldn`t decode", err.Error())
		return NewMemoryDbWithGarbageCollector(defaultExpiration, cleanupInterval, map[string]Item{})
	}

	return NewMemoryDbWithGarbageCollector(defaultExpiration, cleanupInterval, items)
}

func NewMemoryDbWithGarbageCollector(expiration, interval time.Duration, items map[string]Item) *MemoryDB {
	db := newMemoryDB(expiration, items)
	DB := &MemoryDB{db}
	if interval > 0 {
		startGarbageCollector(db, interval)
		runtime.SetFinalizer(db, stopGarbageCollector)
	}
	return DB
}

func newMemoryDB(expiration time.Duration, items map[string]Item) *memoryDB {
	if expiration == 0 {
		expiration = -1
	}
	db := &memoryDB{
		defaultExpiration: expiration,
		items:             items,
	}
	return db
}

// Функция для присвоения значений выбраннному элементу в in-memory db
func (m *memoryDB) SET(key string, value interface{}, d time.Duration) {
	// Устанавливаем время истечения кеша
	var expiration int64

	if d == defaultExpiration {
		d = m.defaultExpiration
	}
	if d > 0 {
		expiration = time.Now().Add(d).UnixNano()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.items[key] = Item{
		Value: value,
		Expiration: expiration,
	}
}

func (m *memoryDB) set(key string, value interface{}, d time.Duration) {
	var expiration int64
	if d == defaultExpiration {
		d = m.defaultExpiration
	}
	if d > 0 {
		expiration = time.Now().Add(d).UnixNano()
	}
	m.items[key] = Item{
		Value:     value,
		Expiration: expiration,
	}
}

// Функция для добавления элемента в in-memory db, с проверкой на существование
func (m *memoryDB) ADD(key string, value interface{}, d time.Duration) error {

	m.mu.Lock()
	defer m.mu.Unlock()

	_, found := m.get(key)
	if found {
		return fmt.Errorf("value with key %s already exists", key)
	}

	m.set(key, value, d)
	return nil
}

// Метод для нахождения значения в in-memory db
func (m *memoryDB) GET(key string) (interface{}, bool) {

	m.mu.RLock()
	defer m.mu.RUnlock()

	item, found := m.items[key]
	if !found {
		return nil, false
	}

	// Проверка на установку времени истечения, в противном случае он бессрочный
	if item.Expiration > 0 {
		// Если в момент запроса кеш устарел возвращаем nil
		if time.Now().UnixNano() > item.Expiration {
			return nil, false
		}
	}

	return item.Value, true
}
func (m *memoryDB) get(key string) (interface{}, bool) {

	item, found := m.items[key]
	if !found {
		return nil, false
	}
	// Проверка на установку времени истечения, в противном случае он бессрочный
	if item.Expiration > 0 {
		// Если в момент запроса кеш устарел возвращаем nil
		if time.Now().UnixNano() > item.Expiration {
			return nil, false
		}
	}

	return item.Value, true
}

// Метод для удаления значения в in-memory db
func (m *memoryDB) DELETE(key string) (bool){
	m.mu.Lock()
	defer m.mu.Unlock()
	value, deleted := m.delete(key)

	if deleted {
		m.onDeleted(key, value)
	}
	return true
}

func (m *memoryDB) delete(key string) (interface{}, bool){
	if m.onDeleted != nil {
		if val, found := m.items[key]; found {
			delete(m.items, key)
			return val.Value, true
		}
	}
	delete(m.items, key)
	return nil, false
}

// Дефолтная функция для onDeleted свойства
func (m *memoryDB) onDeletedDefault(f func(string, interface{})) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onDeleted = f
}

func (m *memoryDB) ExpiredKeysDelete() {
	var deletedItems []keyAndValue
	now := time.Now().UnixNano()
	m.mu.Lock()
	defer m.mu.Unlock()

	for key, item := range m.items {
		if item.Expiration > 0 && now > item.Expiration {
			val, deleted := m.delete(key)
			if deleted {
				deletedItems = append(deletedItems, keyAndValue{key, val})
			}
		}
	}
	for _, val := range deletedItems {
		m.onDeleted(val.key, val.value)
	}
}


// Функция для удаления всех значений из памяти
func (m *memoryDB) clearAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = make(map[string]Item)
	return nil
}


// Сборка мусора или удаление всех элементов с истекшим сроком
func (gc *garbageCollector) startGC(m *memoryDB) {
	ticker := time.NewTicker(gc.Interval)

	for {
		select {
		case <-ticker.C:
			m.ExpiredKeysDelete()
		case <-gc.stop:
			ticker.Stop()
			return
		}
	}
}
func stopGarbageCollector(m *memoryDB) {
	m.garbageCollector.stop <- true
}
func startGarbageCollector(m *memoryDB, interval time.Duration) {
	gc := &garbageCollector{
		Interval: interval,
		stop: make(chan bool),
	}
	m.garbageCollector = gc
	go gc.startGC(m)
}

// Функция для сохранения бэкапа базы локально в файл
func (m *memoryDB) saveToFile() (err error) {
	f, err := os.Create("db.json")
	if err != nil {
		fmt.Println("Couldn`t create file:", err.Error())
		return err
	}

	if err := json.NewEncoder(f).Encode(m.items); err != nil {
		fmt.Println("Couldn`t encode db:", err.Error())
		return err
	}
	f.Close()
	fmt.Println("Successfully saved db to file")
	return f.Close()
}

// Функция загрузки значений из файла
// func (m *memoryDB) loadFromFile() error {
// 	f, err := os.Open("db.json")
// 	if err != nil {
// 		return err
// 	}

// 	err = m.load(f)
// 	if err != nil {
// 		f.Close()
// 		return err
// 	}

// 	return f.Close()
// }

// func (m *memoryDB) load(r io.Reader) error {
// 	items := map[string]Item{}
// 	dec_err := json.NewDecoder(r).Decode(&items)
// 	if dec_err == nil {
// 		m.mu.Lock()
// 		defer m.mu.Unlock()
		
// 		for key, value := range items {
// 			val, found := m.items[key]
// 			if !found || val.isExpire() {
// 				m.items[key] = value
// 			}
// 		}
// 	} 
// 	return dec_err
// }
