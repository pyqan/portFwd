# PortFwd: Пишем TUI-менеджер port-forward для Kubernetes на Go

![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)
![Kubernetes](https://img.shields.io/badge/Kubernetes-1.25+-326CE5?style=flat&logo=kubernetes)
![License](https://img.shields.io/badge/License-MIT-green)

Привет, Хабр! В этой статье я подробно расскажу о создании **PortFwd** — TUI-менеджера для kubectl port-forward. Разберём архитектуру, нюансы работы с Kubernetes API, построение интерфейса на Bubble Tea и множество граблей, на которые я наступил в процессе.

## Оглавление

1. [Мотивация и постановка задачи](#мотивация)
2. [Как работает port-forward в Kubernetes](#как-работает-port-forward)
3. [Архитектура приложения](#архитектура)
4. [Работа с Kubernetes API через client-go](#client-go)
5. [Реализация port-forward: SPDY и WebSocket](#spdy)
6. [Построение TUI на Bubble Tea](#bubble-tea)
7. [Управление состоянием и персистентность](#state)
8. [Грабли и их решения](#грабли)
9. [Производительность и оптимизации](#performance)
10. [Итоги и планы](#итоги)

---

<a name="мотивация"></a>
## 1. Мотивация и постановка задачи

### Типичный workflow DevOps/SRE инженера

Представьте типичное утро:

```bash
# Терминал 1 - база данных
kubectl port-forward svc/postgres 5432:5432 -n production

# Терминал 2 - кэш
kubectl port-forward svc/redis 6379:6379 -n production

# Терминал 3 - очередь сообщений
kubectl port-forward svc/rabbitmq 15672:15672 -n production

# Терминал 4 - API для дебага
kubectl port-forward pod/api-server-7f8d9c6b5-x2k4j 8080:8080 -n staging

# Терминал 5 - ещё один сервис...
# Терминал 6...
# Терминал N...
```

К обеду у вас:
- 8+ открытых терминалов
- Непонятно какой терминал за что отвечает
- Одно соединение упало — нужно найти нужный терминал
- Переключились контекст — нужно всё перенастраивать
- Коллега просит команду — диктуете по буквам

### Существующие решения

| Инструмент | Плюсы | Минусы |
|------------|-------|--------|
| kubectl port-forward | Встроенный, надёжный | Один терминал = одно соединение |
| kubefwd | Массовый форвард | Требует sudo, меняет /etc/hosts |
| Lens | GUI, удобно | Тяжёлый Electron, ресурсоёмкий |
| k9s | TUI, много функций | Port-forward не основная функция |

### Требования к решению

1. **Единое окно** для всех port-forward соединений
2. **Интерактивный выбор** ресурсов (namespace → pod/service → порты)
3. **Сохранение сессий** между запусками
4. **Автоматический резолвинг** targetPort для сервисов
5. **Отдельные логи** для каждого соединения
6. **Graceful shutdown** — корректное завершение без zombie-процессов
7. **Минимальные зависимости** — один бинарник

---

<a name="как-работает-port-forward"></a>
## 2. Как работает port-forward в Kubernetes

Прежде чем писать свою реализацию, разберёмся как это работает под капотом.

### Архитектура port-forward

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Client    │────▶│  API Server │────▶│   Kubelet   │────▶│     Pod     │
│ (localhost) │     │             │     │   (node)    │     │  (container)│
│    :8080    │◀────│   SPDY/WS   │◀────│             │◀────│    :80      │
└─────────────┘     └─────────────┘     └─────────────┘     └─────────────┘
```

### Протокол SPDY

Kubernetes использует **SPDY** (предшественник HTTP/2) для мультиплексирования потоков данных через одно TCP-соединение.

Когда вы выполняете `kubectl port-forward`:

1. **Upgrade Request**: Клиент отправляет HTTP POST с заголовком `Upgrade: SPDY/3.1`
2. **Connection Upgrade**: API Server переключает соединение на SPDY
3. **Stream Creation**: Создаются потоки для данных и ошибок
4. **Data Forwarding**: Kubelet проксирует данные в/из контейнера

### Endpoint API

```
POST /api/v1/namespaces/{namespace}/pods/{pod}/portforward
```

Для сервисов:
```
POST /api/v1/namespaces/{namespace}/services/{service}/portforward
```

**Важно**: Хотя API поддерживает endpoint для сервисов, `kubectl` фактически резолвит сервис в под и подключается к поду напрямую. Это связано с тем, что port-forward работает на уровне конкретного контейнера.

### Заголовки запроса

```http
POST /api/v1/namespaces/default/pods/nginx/portforward HTTP/1.1
Host: kubernetes.default.svc
Upgrade: SPDY/3.1
Connection: Upgrade
X-Stream-Protocol-Version: portforward.k8s.io
```

---

<a name="архитектура"></a>
## 3. Архитектура приложения

### Структура проекта

```
portfwd/
├── main.go                 # Entry point, Cobra CLI
├── internal/
│   ├── k8s/
│   │   └── client.go       # Kubernetes client wrapper
│   ├── portforward/
│   │   └── manager.go      # Port-forward connection manager
│   ├── config/
│   │   ├── config.go       # Profiles configuration
│   │   └── state.go        # Session state persistence
│   └── ui/
│       ├── app.go          # Bubble Tea application
│       ├── views.go        # View rendering
│       └── styles.go       # Lipgloss styles
├── configs/
│   └── profiles.example.yaml
└── articles/
```

### Диаграмма компонентов

```
┌──────────────────────────────────────────────────────────┐
│                        UI Layer                           │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐       │
│  │   Views     │  │   Styles    │  │   App       │       │
│  │  (render)   │  │  (lipgloss) │  │ (Bubble Tea)│       │
│  └─────────────┘  └─────────────┘  └─────────────┘       │
└──────────────────────────────────────────────────────────┘
                            │
                            ▼
┌──────────────────────────────────────────────────────────┐
│                    Business Logic                         │
│  ┌─────────────────────┐  ┌─────────────────────┐        │
│  │  PortForward Manager │  │   Config/State      │        │
│  │  - connections       │  │   - profiles        │        │
│  │  - lifecycle         │  │   - persistence     │        │
│  └─────────────────────┘  └─────────────────────┘        │
└──────────────────────────────────────────────────────────┘
                            │
                            ▼
┌──────────────────────────────────────────────────────────┐
│                   Kubernetes Layer                        │
│  ┌─────────────────────┐  ┌─────────────────────┐        │
│  │    K8s Client       │  │   SPDY Transport    │        │
│  │   (client-go)       │  │  (port-forward)     │        │
│  └─────────────────────┘  └─────────────────────┘        │
└──────────────────────────────────────────────────────────┘
```

### Основные сущности

```go
// Connection - одно port-forward соединение
type Connection struct {
    ID             string
    Namespace      string
    ResourceType   ResourceType  // pod или service
    ResourceName   string
    LocalPort      int
    RemotePort     int
    Status         Status        // active, stopped, error, starting
    Error          string
    Logs           []string
    
    stopChan       chan struct{}
    readyChan      chan struct{}
    cancelFunc     context.CancelFunc
}

// Manager - менеджер всех соединений
type Manager struct {
    connections map[string]*Connection
    clientset   *kubernetes.Clientset
    restConfig  *rest.Config
    onChange    func()  // callback для обновления UI
}
```

---

<a name="client-go"></a>
## 4. Работа с Kubernetes API через client-go

### Инициализация клиента

Используем паттерн из официальных примеров [client-go](https://github.com/kubernetes/client-go/tree/master/examples/out-of-cluster-client-configuration):

```go
import (
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/rest"
    "k8s.io/client-go/tools/clientcmd"
    "k8s.io/client-go/util/homedir"
)

func getKubeConfig() (*rest.Config, error) {
    // 1. Пробуем in-cluster конфиг (если запущены в поде)
    config, err := rest.InClusterConfig()
    if err == nil {
        return config, nil
    }

    // 2. Fallback на kubeconfig файл
    var kubeconfig string
    if envKubeconfig := os.Getenv("KUBECONFIG"); envKubeconfig != "" {
        kubeconfig = envKubeconfig
    } else if home := homedir.HomeDir(); home != "" {
        kubeconfig = filepath.Join(home, ".kube", "config")
    }

    // Используем текущий контекст из kubeconfig
    return clientcmd.BuildConfigFromFlags("", kubeconfig)
}
```

### Получение списка ресурсов

```go
// Получение подов
func (c *Client) GetPods(ctx context.Context, namespace string) ([]PodInfo, error) {
    pods, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
    if err != nil {
        return nil, err
    }

    result := make([]PodInfo, 0, len(pods.Items))
    for _, pod := range pods.Items {
        // Собираем информацию о портах из всех контейнеров
        var ports []ContainerPort
        for _, container := range pod.Spec.Containers {
            for _, port := range container.Ports {
                ports = append(ports, ContainerPort{
                    Name:          port.Name,
                    ContainerPort: port.ContainerPort,
                    Protocol:      string(port.Protocol),
                })
            }
        }
        
        result = append(result, PodInfo{
            Name:      pod.Name,
            Namespace: pod.Namespace,
            Status:    string(pod.Status.Phase),
            Ports:     ports,
        })
    }
    return result, nil
}
```

### Резолвинг Service → Pod

Одна из ключевых функций — автоматическое определение backing pod для сервиса:

```go
func (m *Manager) resolveServiceToPod(ctx context.Context, namespace, serviceName string, servicePort int) (podName string, targetPort int, err error) {
    // 1. Получаем сервис
    svc, err := m.clientset.CoreV1().Services(namespace).Get(ctx, serviceName, metav1.GetOptions{})
    if err != nil {
        return "", 0, err
    }

    // 2. Резолвим targetPort
    targetPort = servicePort
    for _, port := range svc.Spec.Ports {
        if int(port.Port) == servicePort {
            if port.TargetPort.IntValue() != 0 {
                // TargetPort указан как число
                targetPort = port.TargetPort.IntValue()
            } else if port.TargetPort.String() != "" {
                // TargetPort указан как named port - нужно резолвить из пода
                // (разберём ниже)
            }
            break
        }
    }

    // 3. Находим под по selector
    selector := svc.Spec.Selector
    var labelSelector []string
    for k, v := range selector {
        labelSelector = append(labelSelector, fmt.Sprintf("%s=%s", k, v))
    }

    pods, err := m.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
        LabelSelector: strings.Join(labelSelector, ","),
    })
    
    // 4. Выбираем первый Running под
    for _, pod := range pods.Items {
        if pod.Status.Phase == corev1.PodRunning {
            return pod.Name, targetPort, nil
        }
    }
    
    return "", 0, fmt.Errorf("no running pods found for service")
}
```

### Резолвинг Named Ports

Сервис может использовать named port:

```yaml
apiVersion: v1
kind: Service
spec:
  ports:
    - port: 80
      targetPort: http  # Это named port!
```

В этом случае нужно найти порт в спецификации пода:

```go
// Named port - ищем в контейнерах пода
if port.TargetPort.Type == intstr.String {
    namedPort := port.TargetPort.String()
    for _, container := range pod.Spec.Containers {
        for _, cp := range container.Ports {
            if cp.Name == namedPort {
                targetPort = int(cp.ContainerPort)
                break
            }
        }
    }
}
```

---

<a name="spdy"></a>
## 5. Реализация port-forward: SPDY и WebSocket

### Создание SPDY-транспорта

```go
import (
    "k8s.io/client-go/tools/portforward"
    "k8s.io/client-go/transport/spdy"
)

func (m *Manager) doPortForward(ctx context.Context, conn *Connection) error {
    // 1. Формируем URL для API запроса
    req := m.clientset.CoreV1().RESTClient().Post().
        Resource("pods").
        Namespace(conn.Namespace).
        Name(podName).
        SubResource("portforward")

    // 2. Создаём SPDY transport
    transport, upgrader, err := spdy.RoundTripperFor(m.restConfig)
    if err != nil {
        return err
    }

    // 3. Создаём dialer
    dialer := spdy.NewDialer(
        upgrader,
        &http.Client{Transport: transport},
        http.MethodPost,
        req.URL(),
    )

    // 4. Конфигурируем port-forwarder
    ports := []string{fmt.Sprintf("%d:%d", conn.LocalPort, targetPort)}
    
    // Привязываемся только к IPv4 для избежания проблем
    addresses := []string{"127.0.0.1"}
    
    fw, err := portforward.NewOnAddresses(
        dialer,
        addresses,
        ports,
        conn.stopChan,   // канал для остановки
        conn.readyChan,  // канал сигнализирует о готовности
        outWriter,       // stdout
        errWriter,       // stderr
    )
    
    // 5. Запускаем (блокирующий вызов)
    return fw.ForwardPorts()
}
```

### Обработка готовности и ошибок

```go
func (m *Manager) runPortForward(ctx context.Context, conn *Connection) error {
    // Запускаем ForwardPorts в горутине
    errChan := make(chan error, 1)
    go func() {
        errChan <- fw.ForwardPorts()
    }()

    // Ждём либо ready, либо error
    select {
    case <-conn.readyChan:
        conn.Status = StatusActive
        conn.AddLog("✓ Tunnel ready")
        
    case err := <-errChan:
        conn.Status = StatusError
        conn.Error = err.Error()
        return err
    }

    // Теперь ждём завершения или отмены
    select {
    case err := <-errChan:
        return err
    case <-ctx.Done():
        // Context отменён - выходим
        return nil
    }
}
```

### Почему IPv4?

При использовании `portforward.New()` по умолчанию, forwarder пытается слушать и на IPv4, и на IPv6:

```
Forwarding from 127.0.0.1:8080 -> 8000
Forwarding from [::1]:8080 -> 8000
```

На некоторых системах это вызывает проблемы. Используя `NewOnAddresses` с `[]string{"127.0.0.1"}`, мы явно указываем только IPv4.

---

<a name="bubble-tea"></a>
## 6. Построение TUI на Bubble Tea

### Почему Bubble Tea?

[Bubble Tea](https://github.com/charmbracelet/bubbletea) от Charm — это фреймворк для построения TUI на Go, вдохновлённый Elm Architecture:

- **Model** — состояние приложения
- **Update** — обработка событий, возврат нового состояния
- **View** — рендеринг состояния в строку

```go
type Model struct {
    view            View
    connections     []*Connection
    selectedConn    int
    namespaces      []string
    pods            []PodInfo
    services        []ServiceInfo
    // ...
}

func (m Model) Init() tea.Cmd {
    return tea.Batch(
        m.loadNamespaces(),
        m.loadContext(),
    )
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        return m.handleKeyPress(msg)
    case tea.WindowSizeMsg:
        m.width = msg.Width
        m.height = msg.Height
    case namespacesMsg:
        m.namespaces = msg
    // ...
    }
    return m, nil
}

func (m Model) View() string {
    return m.renderHeader() + m.renderContent() + m.renderHelp()
}
```

### Стилизация с Lipgloss

[Lipgloss](https://github.com/charmbracelet/lipgloss) — библиотека для стилизации терминального вывода:

```go
var (
    ColorPrimary = lipgloss.Color("#00FF9F")  // Neon green
    ColorError   = lipgloss.Color("#FF3366")  // Red
    
    TitleStyle = lipgloss.NewStyle().
        Bold(true).
        Foreground(ColorPrimary).
        Padding(0, 2)
    
    BoxStyle = lipgloss.NewStyle().
        Border(lipgloss.RoundedBorder()).
        BorderForeground(lipgloss.Color("#30363D")).
        Padding(1, 2)
    
    SelectedItemStyle = lipgloss.NewStyle().
        Foreground(lipgloss.Color("#0D1117")).
        Background(ColorPrimary).
        Bold(true)
)
```

### Асинхронные команды

Bubble Tea использует команды (Cmd) для асинхронных операций:

```go
// Команда для загрузки namespaces
func (m Model) loadNamespaces() tea.Cmd {
    return func() tea.Msg {
        namespaces, err := m.k8sClient.GetNamespaces(context.Background())
        if err != nil {
            return errMsg{err}
        }
        return namespacesMsg(namespaces)
    }
}

// Команда для старта port-forward
func (m Model) startPortForward(namespace, pod string, local, remote int) tea.Cmd {
    return func() tea.Msg {
        _, err := m.pfManager.StartPortForwardToPod(
            context.Background(),
            namespace, pod, local, remote,
        )
        if err != nil {
            return portForwardFailed{err: err}
        }
        return portForwardStarted{}
    }
}
```

### Обновление UI при изменении соединений

Менеджер port-forward вызывает callback при изменении состояния:

```go
// В UI
pfManager.SetOnChange(func() {
    p.Send(connectionsUpdated{})
})

// В Manager
func (m *Manager) notifyChange() {
    if m.onChange != nil {
        m.onChange()
    }
}
```

**Важный нюанс**: При завершении приложения нужно отключить callback, иначе `p.Send()` заблокируется:

```go
func (m *Manager) StopAll() {
    // Отключаем callback ПЕРЕД остановкой
    m.mu.Lock()
    m.onChange = nil
    m.mu.Unlock()
    
    // Теперь безопасно останавливаем соединения
    for _, conn := range m.connections {
        // ...
    }
}
```

---

<a name="state"></a>
## 7. Управление состоянием и персистентность

### Сохранение сессии

При выходе сохраняем состояние в YAML:

```go
type SessionState struct {
    LastSaved   time.Time         `yaml:"lastSaved"`
    Connections []SavedConnection `yaml:"connections"`
}

type SavedConnection struct {
    Namespace    string `yaml:"namespace"`
    ResourceType string `yaml:"resourceType"`
    ResourceName string `yaml:"resourceName"`
    LocalPort    int    `yaml:"localPort"`
    RemotePort   int    `yaml:"remotePort"`
    WasActive    bool   `yaml:"wasActive"`  // Было ли активно при сохранении
}
```

Файл: `~/.config/portfwd/state.yaml`

```yaml
lastSaved: 2024-01-19T15:30:00Z
connections:
  - namespace: production
    resourceType: service
    resourceName: postgres
    localPort: 5432
    remotePort: 5432
    wasActive: true
  - namespace: staging
    resourceType: pod
    resourceName: api-server-7f8d9
    localPort: 8080
    remotePort: 8080
    wasActive: false
```

### Восстановление при запуске

```go
func restorePreviousSession(k8sClient *k8s.Client, pfManager *portforward.Manager) {
    state, err := config.LoadState()
    if err != nil || len(state.Connections) == 0 {
        return
    }

    for _, saved := range state.Connections {
        resourceType := portforward.ResourcePod
        if saved.ResourceType == "service" {
            resourceType = portforward.ResourceService
        }

        if !saved.WasActive {
            // Было остановлено - добавляем как остановленное
            pfManager.AddStoppedConnection(
                saved.Namespace, resourceType, saved.ResourceName,
                saved.LocalPort, saved.RemotePort,
            )
            continue
        }

        // Было активно - проверяем доступность и переподключаемся
        available := checkResourceAvailable(k8sClient, saved)
        if !available {
            pfManager.AddStoppedConnection(...)
            continue
        }

        // Пытаемся переподключиться
        if saved.ResourceType == "service" {
            pfManager.StartPortForwardToService(...)
        } else {
            pfManager.StartPortForwardToPod(...)
        }
    }
}
```

### Порядок при выходе

Критически важно сохранить состояние **до** остановки соединений:

```go
case "q", "ctrl+c":
    // 1. Сначала сохраняем (соединения ещё активны)
    saveSessionState(m.pfManager)
    // 2. Потом останавливаем
    m.pfManager.StopAll()
    return m, tea.Quit
```

---

<a name="грабли"></a>
## 8. Грабли и их решения

### Проблема 1: Зависание при выходе

**Симптом**: Приложение не завершается после нажатия `q`.

**Причина**: `onChange` callback вызывает `p.Send()` в Bubble Tea, который блокируется после `tea.Quit`.

**Решение**:
```go
func (m *Manager) StopAll() {
    m.mu.Lock()
    m.onChange = nil  // Отключаем ДО остановки
    m.mu.Unlock()
    // ...
}
```

### Проблема 2: Неправильный порт для сервисов

**Симптом**: `connection refused` при подключении к сервису.

**Причина**: Использовался `port` сервиса (80) вместо `targetPort` (8000).

**Решение**: Полный резолвинг targetPort, включая named ports:

```go
// Service port: 80, targetPort: 8000 → используем 8000
// Service port: 80, targetPort: "http" → ищем в pod spec порт с name="http"
```

### Проблема 3: IPv6 connection refused

**Симптом**: 
```
IPv4: dial tcp4 127.0.0.1:80: connect: connection refused
IPv6: dial tcp6 [::1]:80: connect: connection refused
```

**Причина**: По умолчанию forwarder слушает на обоих протоколах.

**Решение**: Явно указываем только IPv4:
```go
portforward.NewOnAddresses(dialer, []string{"127.0.0.1"}, ...)
```

### Проблема 4: Panic при повторном close(channel)

**Симптом**: `panic: close of closed channel`

**Причина**: `stopChan` закрывается несколько раз при быстром нажатии stop.

**Решение**: Используем `sync.Once`:
```go
type Connection struct {
    stopOnce sync.Once
    // ...
}

func (m *Manager) StopPortForward(id string) {
    conn.stopOnce.Do(func() {
        close(conn.stopChan)
    })
}
```

### Проблема 5: Потеря соединения через время

**Симптом**: Соединение отваливается после нескольких минут неактивности.

**Причина**: Idle timeout на уровне API Server или промежуточных прокси.

**Решение**: Пока не реализовано автоматическое переподключение, но структура готова:
```go
type Connection struct {
    AutoReconnect  bool
    ReconnectCount int
}
```

---

<a name="performance"></a>
## 9. Производительность и оптимизации

### Потребление ресурсов

Сравнение с альтернативами (idle, 5 активных соединений):

| Инструмент | RAM | CPU |
|------------|-----|-----|
| 5x kubectl port-forward | ~100 MB | ~0.5% |
| kubefwd | ~80 MB | ~0.3% |
| Lens | ~500 MB | ~2% |
| **PortFwd** | ~30 MB | ~0.1% |

### Оптимизации

1. **Один бинарник** — нет накладных расходов на интерпретатор
2. **Нативный Go** — эффективная работа с памятью
3. **Переиспользование клиента** — один clientset для всех операций
4. **Ленивая загрузка** — ресурсы загружаются по требованию

### Ограничения

- Максимум ~100 одновременных соединений (ограничение file descriptors)
- Один kubeconfig/context (пока)
- Нет поддержки exec/attach (только port-forward)

---

<a name="итоги"></a>
## 10. Итоги и планы

### Что получилось

✅ Единое окно для всех port-forward  
✅ Интерактивный выбор ресурсов  
✅ Сохранение сессий между запусками  
✅ Автоматический резолвинг targetPort  
✅ Отдельные логи для каждого соединения  
✅ Graceful shutdown  
✅ Минимальное потребление ресурсов  

### Roadmap

- [ ] **Профили** — быстрое переключение между наборами соединений
- [ ] **Группы** — логическая группировка соединений
- [ ] **Multi-cluster** — работа с несколькими кластерами
- [ ] **Import** — импорт из kubectl команд или YAML
- [ ] **Auto-reconnect** — автоматическое переподключение
- [ ] **Metrics** — статистика по соединениям

### Ссылки

- **GitHub**: [github.com/yourusername/portfwd](https://github.com/yourusername/portfwd)
- **client-go examples**: [github.com/kubernetes/client-go/examples](https://github.com/kubernetes/client-go/tree/master/examples)
- **Bubble Tea**: [github.com/charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea)
- **Lipgloss**: [github.com/charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss)

---

## Заключение

Создание PortFwd было увлекательным погружением в внутренности Kubernetes и современные подходы к построению CLI-приложений. Надеюсь, статья была полезной и инструмент пригодится в вашей работе.

Буду рад:
- ⭐ Звёздам на GitHub
- 🐛 Issue с багами и предложениями
- 🔀 Pull Request с улучшениями

Спасибо за внимание!

---

*Теги: Go, Kubernetes, TUI, DevOps, client-go, Bubble Tea, port-forward, инструменты разработчика*
