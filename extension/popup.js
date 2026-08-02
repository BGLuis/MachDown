// --- Controle de Telas ---
// Flag que indica se o usuário veio da tela principal (via botão ⚙️).
// Quando true, o botão "← Voltar" é exibido na tela de configuração.
let cameFromMain = false;

function showScreen(name) {
    document.getElementById('screen-setup').style.display = name === 'setup' ? 'block' : 'none';
    document.getElementById('screen-main').style.display  = name === 'main'  ? 'block' : 'none';
}

// --- Tela de Configuração ---
function loadConfigIntoForm(items) {
    document.getElementById('server_url').value = items.server_url || 'http://localhost:8888';
    document.getElementById('api_key').value    = items.api_key    || '';
    
    const interceptCheckbox = document.getElementById('intercept_enabled');
    interceptCheckbox.checked = items.intercept_enabled !== false; // Padrão é true
    
    // Configura o client_id se já existir
    const clientSelect = document.getElementById('client_id');
    if (items.client_id) {
        let exists = false;
        for (let i = 0; i < clientSelect.options.length; i++) {
            if (clientSelect.options[i].value === items.client_id) {
                exists = true;
                break;
            }
        }
        if (!exists) {
            const opt = document.createElement('option');
            opt.value = items.client_id;
            opt.text = items.client_id;
            clientSelect.add(opt);
        }
        clientSelect.value = items.client_id;
    }
}

document.getElementById('btn-fetch-clients').addEventListener('click', () => {
    const server_url = document.getElementById('server_url').value.trim().replace(/\/$/, '');
    const api_key    = document.getElementById('api_key').value.trim();
    const status = document.getElementById('status');

    if (!server_url || !api_key) {
        status.textContent = '⚠️ Informe Server URL e API Key primeiro.';
        return;
    }

    status.textContent = 'Buscando clientes...';
    
    fetch(`${server_url}/api/admin/clients`, {
        headers: { 'X-API-Key': api_key }
    })
    .then(res => {
        if (!res.ok) throw new Error(`Erro ${res.status}`);
        return res.json();
    })
    .then(clients => {
        const clientSelect = document.getElementById('client_id');
        const currentVal = clientSelect.value;
        clientSelect.innerHTML = '<option value="">Selecione...</option>';
        
        clients.forEach(c => {
            const opt = document.createElement('option');
            opt.value = c.id;
            opt.text = c.id;
            clientSelect.add(opt);
        });
        
        if (currentVal && clients.find(c => c.id === currentVal)) {
            clientSelect.value = currentVal;
        } else if (clients.length > 0) {
            clientSelect.value = clients[0].id; // Auto select first
        }
        
        status.textContent = '✅ Clientes carregados!';
        setTimeout(() => status.textContent = '', 2000);
    })
    .catch(err => {
        status.textContent = '❌ Erro ao buscar: ' + err.message;
    });
});

document.getElementById('save').addEventListener('click', () => {
    const server_url = document.getElementById('server_url').value.trim().replace(/\/$/, '');
    const api_key    = document.getElementById('api_key').value.trim();
    const client_id  = document.getElementById('client_id').value.trim();
    const intercept_enabled = document.getElementById('intercept_enabled').checked;

    if (!server_url || !api_key || !client_id) {
        document.getElementById('status').textContent = '⚠️ Preencha todos os campos.';
        return;
    }

    chrome.storage.sync.set({ server_url, api_key, client_id, intercept_enabled }, () => {
        const status = document.getElementById('status');
        status.textContent = '✅ Salvo! Conectando...';
        setTimeout(() => {
            status.textContent = '';
            cameFromMain = false;
            document.getElementById('btn-back').style.display = 'none';
            renderMainScreen({ server_url, api_key, client_id });
            showScreen('main');
        }, 1200);
    });
});

// Botão "← Voltar": descarta alterações e retorna ao dashboard sem salvar.
document.getElementById('btn-back').addEventListener('click', () => {
    cameFromMain = false;
    document.getElementById('btn-back').style.display = 'none';
    document.getElementById('status').textContent = '';
    showScreen('main');
});

// --- Tela Principal ---
function renderMainScreen(items) {
    const serverEl = document.getElementById('display-server');
    if (serverEl) serverEl.textContent = items.server_url || '—';
    fetchDownloads(items);
}

function fetchDownloads(items) {
    const list = document.getElementById('ext-downloads-list');
    list.innerHTML = '<li style="color:#64748b; text-align:center;">Carregando...</li>';
    
    fetch(`${items.server_url}/api/admin/jobs`, {
        headers: {
            'X-API-Key': items.api_key,
            'X-Client-ID': items.client_id
        }
    })
    .then(res => res.json())
    .then(jobs => {
        if (!jobs || jobs.length === 0) {
            list.innerHTML = '<li style="color:#64748b; text-align:center;">Nenhum download</li>';
            return;
        }
        
        // Ordena por data (mais recentes primeiro) e pega os top 10
        jobs.sort((a, b) => new Date(b.created_at) - new Date(a.created_at));
        const recent = jobs.slice(0, 10);
        
        list.innerHTML = recent.map(j => {
            const isTarget = (j.target_clients || '').includes(items.client_id);
            const statusColor = j.status === 'Completed' ? '#4ade80' : (j.status === 'Error' ? '#ef4444' : '#facc15');
            return `
                <li style="border-bottom: 1px solid #2e4460; padding: 6px 0;">
                    <div style="white-space: nowrap; overflow: hidden; text-overflow: ellipsis; color: #e2e8f0;">
                        ${j.file_name || j.url}
                    </div>
                    <div style="display:flex; justify-content:space-between; margin-top:2px;">
                        <span style="color: ${statusColor}; font-weight:bold;">${j.status}</span>
                        ${isTarget ? '<span style="color:#38bdf8;">(Para você)</span>' : ''}
                    </div>
                </li>
            `;
        }).join('');
    })
    .catch(err => {
        list.innerHTML = `<li style="color:#ef4444; text-align:center;">Erro: ${err.message}</li>`;
    });
}

document.getElementById('btn-refresh-downloads').addEventListener('click', () => {
    chrome.storage.sync.get(['server_url', 'api_key', 'client_id', 'intercept_enabled'], (items) => {
        fetchDownloads(items);
    });
});

document.getElementById('btn-direct-download').addEventListener('click', () => {
    const urlInput = document.getElementById('direct_url');
    const url = urlInput.value.trim();
    if (!url) return;
    
    chrome.storage.sync.get(['server_url', 'api_key', 'client_id'], (items) => {
        const btn = document.getElementById('btn-direct-download');
        btn.textContent = '⏳';
        
        fetch(`${items.server_url}/api/downloads`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'X-API-Key': items.api_key,
                'X-Client-ID': items.client_id
            },
            body: JSON.stringify({
                url: url,
                target_clients: [items.client_id],
                user_agent: navigator.userAgent
            })
        })
        .then(res => {
            if (!res.ok) throw new Error('Falha ao enviar');
            urlInput.value = '';
            fetchDownloads(items);
        })
        .catch(err => alert(err.message))
        .finally(() => {
            btn.textContent = '🚀';
        });
    });
});

// Botão ⚙️: abre configurações a partir da tela principal.
// Mostra o botão "Voltar" pois há uma tela para retornar.
document.getElementById('btn-settings').addEventListener('click', () => {
    chrome.storage.sync.get(['server_url', 'api_key', 'client_id', 'intercept_enabled'], (items) => {
        cameFromMain = true;
        document.getElementById('btn-back').style.display = 'inline-block';
        document.getElementById('status').textContent = '';
        loadConfigIntoForm(items);
        showScreen('setup');
    });
});

// --- Init: decide qual tela mostrar ---
// No primeiro acesso (sem config salva) → setup sem botão Voltar.
// Já configurado → dashboard direto.
document.addEventListener('DOMContentLoaded', () => {
    chrome.storage.sync.get(['server_url', 'api_key', 'client_id', 'intercept_enabled'], (items) => {
        const isConfigured = !!(items.api_key && items.server_url && items.client_id);
        if (isConfigured) {
            renderMainScreen(items);
            showScreen('main');
        } else {
            cameFromMain = false;
            document.getElementById('btn-back').style.display = 'none';
            loadConfigIntoForm(items);
            showScreen('setup');
        }
    });
});
