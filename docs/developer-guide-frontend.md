# Frontend Developer Guide

## Overview

The LLMSee frontend is a Vue 3 application that provides real-time monitoring of LLM API requests. It uses Server-Sent Events (SSE) for live updates and focuses on simplicity and performance.

## Technology Stack

- **Vue 3** - Progressive JavaScript framework (CDN version 3.5.13)
- **HTML5** - Semantic markup with Vue templates
- **CSS3** - Modern styling with Skeleton CSS framework
- **Prism.js** - Syntax highlighting for JSON display
- **Server-Sent Events** - Real-time updates from backend
- **Embedded Assets** - UI files are embedded in the Go binary for production

## File Structure

```
ui/
├── index.html          # Vue 3 application with embedded JavaScript
├── styles.css          # Application styles (extends Skeleton CSS)
└── favicon.ico         # Application icon
```

## Vue 3 Application Structure

The entire Vue application is contained within `index.html` using the CDN version of Vue 3. The application uses:

- **Vue 3 Composition API** - Modern Vue development approach
- **CDN Dependencies** - Vue 3, Prism.js, and Skeleton CSS loaded from CDN
- **Single File Structure** - All Vue code embedded in the HTML file
- **Reactive Data** - Vue's reactivity system for real-time UI updates

## Development Setup

### Development Mode
```bash
APP_ENV=dev ./llmsee
```

In development mode:
- UI files are served from the filesystem (`ui/` directory)
- Changes are reflected immediately without rebuilding
- No need to restart the server for UI changes

### Production Mode
```bash
./llmsee
```

In production mode:
- UI files are embedded in the binary via `//go:embed`
- Faster startup and deployment
- Single binary distribution

## Architecture Overview

### Vue Components and Features

1. **Request Log Table** - Reactive table displaying paginated API requests
2. **Request Detail Modal** - Modal component for full request/response data
3. **Real-time Updates** - SSE integration with Vue's reactivity
4. **Pagination Controls** - Vue-powered navigation controls
5. **Markers** - Visual markers to separate request groups
6. **Models View** - Display available models from all providers

### Vue Data Flow

```
Backend SSE → Vue Event Handler → Reactive Data Update → Template Re-render
```

1. Backend sends SSE events when new requests are logged
2. Vue receives events and updates reactive data properties
3. Vue automatically re-renders affected template sections
4. User interactions trigger Vue methods that update state

## Key Vue Components

### Vue App Structure

```javascript
const { createApp, ref, computed, onMounted, onUnmounted } = Vue;

createApp({
    setup() {
        // Reactive data
        const logs = ref([]);
        const currentPage = ref(1);
        const totalPages = ref(1);
        const loading = ref(true);
        const selectedLogId = ref(null);
        
        // Computed properties
        const logsWithMarkers = computed(() => {
            // Process logs with visual markers
        });
        
        // Methods
        const fetchLogs = (page) => {
            // Fetch paginated logs
        };
        
        const openLogDetail = (id) => {
            // Show detailed log view
        };
        
        return {
            logs,
            currentPage,
            totalPages,
            loading,
            selectedLogId,
            logsWithMarkers,
            fetchLogs,
            openLogDetail
        };
    }
}).mount('#app');
```

### SSE Integration with Vue

```javascript
// Within Vue setup()
let eventSource = null;

const connectSSE = () => {
    eventSource = new EventSource('/ui/sse');
    
    eventSource.onmessage = (event) => {
        const data = JSON.parse(event.data);
        // Update reactive data - Vue handles UI updates automatically
        if (currentPage.value === 1) {
            logs.value.unshift(data);
            // Add flash effect
            data._flash = true;
            setTimeout(() => data._flash = false, 1000);
        }
    };
    
    eventSource.onerror = () => {
        console.error('SSE connection lost, attempting reconnect...');
        setTimeout(connectSSE, 5000);
    };
};

onMounted(() => {
    connectSSE();
    fetchLogs(1);
});

onUnmounted(() => {
    if (eventSource) {
        eventSource.close();
    }
});
```

### Request Table Management

```javascript
// Update table with new request data
function updateRequestTable(requestData) {
    // Add new row to table
    // Update existing rows if needed
    // Maintain sort order (newest first)
}

// Handle pagination
function loadPage(pageNumber) {
    fetch(`/log?page=${pageNumber}&pageSize=${pageSize}`)
        .then(response => response.json())
        .then(data => renderRequestTable(data));
}
```

### Modal Dialog Handling

```javascript
// Show request details in modal
function showRequestDetail(requestId) {
    fetch(`/log/detail?id=${requestId}`)
        .then(response => response.json())
        .then(data => renderRequestDetail(data));
}

// Close modal and cleanup
function closeModal() {
    // Hide modal
    // Clear content
    // Remove event listeners
}
```

## Styling Guidelines

### CSS Architecture

The application uses a single CSS file (`styles.css`) organized into sections:

1. **Reset/Base Styles** - Normalize browser defaults
2. **Layout Components** - Main layout, containers
3. **Table Styles** - Request log table styling
4. **Modal Styles** - Detail modal dialog
5. **Utility Classes** - Reusable helper classes
6. **Responsive Design** - Mobile-friendly adaptations

### Design Principles

- **Dark Theme** - Easy on the eyes for monitoring
- **Monospace Fonts** - Better for code and JSON display
- **High Contrast** - Ensure readability
- **Minimal UI** - Focus on the data, not decoration
- **Responsive** - Works on desktop and mobile devices

### Color Scheme

```css
:root {
    --bg-primary: #1a1a1a;
    --bg-secondary: #2d2d2d;
    --text-primary: #ffffff;
    --text-secondary: #cccccc;
    --accent: #4a9eff;
    --success: #28a745;
    --warning: #ffc107;
    --error: #dc3545;
}
```

## Real-time Features

### Server-Sent Events Implementation

```javascript
class SSEManager {
    constructor() {
        this.eventSource = null;
        this.reconnectAttempts = 0;
        this.maxReconnectAttempts = 5;
    }
    
    connect() {
        this.eventSource = new EventSource('/ui/sse');
        
        this.eventSource.onopen = () => {
            console.log('SSE connection established');
            this.reconnectAttempts = 0;
        };
        
        this.eventSource.onmessage = (event) => {
            this.handleMessage(JSON.parse(event.data));
        };
        
        this.eventSource.onerror = () => {
            this.handleError();
        };
    }
    
    handleMessage(data) {
        // Update UI with new request data
        this.updateRequestList(data);
    }
    
    handleError() {
        this.eventSource.close();
        if (this.reconnectAttempts < this.maxReconnectAttempts) {
            setTimeout(() => {
                this.reconnectAttempts++;
                this.connect();
            }, 1000 * this.reconnectAttempts);
        }
    }
}
```

### Live Updates

- **New Requests** - Automatically appear at the top of the list
- **Status Updates** - Request status changes are reflected immediately
- **Connection Status** - Visual indicator of SSE connection health
- **Auto-refresh** - Fallback polling if SSE connection fails

## Data Display

### Request Table Columns

| Column | Description | Format |
|--------|-------------|---------|
| Timestamp | When the request was made | Relative time (e.g., "2 minutes ago") |
| Provider | Which LLM provider | Badge with provider name |
| Model | Model name used | Text, truncated if long |
| Method | HTTP method | GET/POST badge |
| Status | Response status | Color-coded (green=2xx, red=4xx/5xx) |
| Duration | Request duration | Milliseconds |

### Request Detail Modal

```javascript
// Structure for detailed request view
const requestDetail = {
    basic: {
        id: 'Request ID',
        timestamp: 'Full timestamp',
        provider: 'Provider name',
        model: 'Model name',
        method: 'HTTP method',
        status: 'Response status',
        duration: 'Duration in ms'
    },
    request: {
        url: 'Target URL',
        headers: 'JSON formatted headers',
        body: 'Formatted request body'
    },
    response: {
        headers: 'JSON formatted headers',
        body: 'Formatted response body'
    }
};
```

### JSON Formatting

```javascript
function formatJSON(jsonString) {
    try {
        const parsed = JSON.parse(jsonString);
        return JSON.stringify(parsed, null, 2);
    } catch (e) {
        return jsonString; // Return original if not valid JSON
    }
}

function syntaxHighlight(json) {
    // Add syntax highlighting for JSON display
    return json.replace(
        /("(\\u[a-zA-Z0-9]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(true|false|null)\b|-?\d+(?:\.\d*)?(?:[eE][+\-]?\d+)?)/g,
        function (match) {
            // Apply appropriate CSS classes for highlighting
        }
    );
}
```

## Performance Optimizations

### Efficient DOM Updates

```javascript
// Use document fragments for batch updates
function updateRequestTable(requests) {
    const fragment = document.createDocumentFragment();
    
    requests.forEach(request => {
        const row = createRequestRow(request);
        fragment.appendChild(row);
    });
    
    // Single DOM update
    tableBody.appendChild(fragment);
}

// Virtualization for large datasets
function renderVisibleRows() {
    // Only render rows currently visible in viewport
    // Implement virtual scrolling for performance
}
```

### Memory Management

```javascript
// Clean up event listeners
function cleanup() {
    if (this.eventSource) {
        this.eventSource.close();
        this.eventSource = null;
    }
    
    // Remove DOM event listeners
    document.removeEventListener('click', this.handleClick);
}

// Limit stored data
function maintainDataLimit() {
    // Keep only last N requests in memory
    // Implement LRU cache for request details
}
```

## Responsive Design

### Mobile Adaptations

```css
@media (max-width: 768px) {
    .request-table {
        /* Stack table cells vertically */
        display: block;
    }
    
    .request-row {
        /* Card-like layout for mobile */
        display: block;
        border: 1px solid var(--border-color);
        margin-bottom: 1rem;
    }
    
    .modal-content {
        /* Full-screen modal on mobile */
        width: 100vw;
        height: 100vh;
        margin: 0;
    }
}
```

### Touch-Friendly Interface

- **Larger Touch Targets** - Minimum 44px for interactive elements
- **Swipe Gestures** - Consider adding swipe navigation
- **Scroll Performance** - Optimize for smooth scrolling
- **Viewport Meta Tag** - Proper mobile viewport configuration

## Adding New Features

### Adding a New Column

1. **Update HTML** - Add column header and cell template
2. **Update CSS** - Style the new column appropriately
3. **Update JavaScript** - Include data in row creation
4. **Test Responsive** - Ensure mobile layout still works

### Adding Filtering

```javascript
class FilterManager {
    constructor() {
        this.filters = {
            provider: null,
            model: null,
            status: null,
            dateRange: null
        };
    }
    
    applyFilters(requests) {
        return requests.filter(request => {
            return this.matchesProvider(request) &&
                   this.matchesModel(request) &&
                   this.matchesStatus(request) &&
                   this.matchesDateRange(request);
        });
    }
    
    updateUI() {
        const filteredRequests = this.applyFilters(allRequests);
        renderRequestTable(filteredRequests);
    }
}
```

### Adding Search

```javascript
class SearchManager {
    constructor() {
        this.searchTerm = '';
        this.searchFields = ['provider', 'model', 'request_body', 'response_body'];
    }
    
    search(term) {
        this.searchTerm = term.toLowerCase();
        const results = allRequests.filter(request => {
            return this.searchFields.some(field => {
                const value = request[field] || '';
                return value.toLowerCase().includes(this.searchTerm);
            });
        });
        renderRequestTable(results);
    }
}
```

## Testing Frontend Code

### Manual Testing Checklist

- [ ] SSE connection establishes correctly
- [ ] New requests appear in real-time
- [ ] Pagination works correctly
- [ ] Modal opens and closes properly
- [ ] Request details display correctly
- [ ] Responsive design works on mobile
- [ ] Error handling works (disconnect SSE, network errors)

### Browser Compatibility

- **Modern Browsers** - Chrome 60+, Firefox 55+, Safari 12+, Edge 79+
- **ES6+ Features** - Arrow functions, const/let, template literals
- **SSE Support** - All modern browsers support Server-Sent Events
- **Fallback Strategy** - Polling fallback for older browsers if needed

### Performance Testing

```javascript
// Measure rendering performance
function measureRenderTime() {
    const start = performance.now();
    renderRequestTable(largeDataset);
    const end = performance.now();
    console.log(`Render time: ${end - start}ms`);
}

// Monitor memory usage
function checkMemoryUsage() {
    if (performance.memory) {
        console.log('Memory usage:', performance.memory);
    }
}
```

## Debugging Tips

### Browser Developer Tools

1. **Network Tab** - Monitor SSE connection and API calls
2. **Console** - Check for JavaScript errors and logs
3. **Elements Tab** - Inspect DOM structure and CSS
4. **Performance Tab** - Profile rendering performance
5. **Application Tab** - Check SSE connection status

### Common Issues

1. **SSE Connection Drops** - Check network stability, implement reconnection
2. **Memory Leaks** - Monitor memory usage, clean up event listeners
3. **Slow Rendering** - Profile DOM updates, implement virtualization
4. **Mobile Layout Issues** - Test on actual devices, not just browser resize

### Debug Utilities

```javascript
// Debug SSE events
function debugSSE() {
    const eventSource = new EventSource('/ui/sse');
    eventSource.onmessage = function(event) {
        console.log('SSE Event:', event.data);
    };
}

// Performance monitoring
function monitorPerformance() {
    const observer = new PerformanceObserver((list) => {
        list.getEntries().forEach((entry) => {
            console.log('Performance:', entry);
        });
    });
    observer.observe({entryTypes: ['measure', 'navigation']});
}
```