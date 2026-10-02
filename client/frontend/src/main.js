import { GetConfig, SaveConfig, GetJobs, IsConfigured, ListServerFiles, DeleteServerFile, GetServerConfig, UpdateServerConfig, CleanServer, DeleteBatchServerFiles, ForceSyncBatchServerFiles, ListAPIKeys, CreateAPIKey, DeleteAPIKey, ListClients } from '../wailsjs/go/main/App';

// --- Controle de Telas ---
// Flag que indica se o usuário veio da tela principal (via botão ⚙️).
// Quando true, o botão "Voltar" é exibido na tela de configuração.
let cameFromMain = false;

function showScreen(name) {
    document.getElementById('screen-setup').style.display = name === 'setup' ? 'block' : 'none';
    document.getElementById('screen-main').style.display = name === 'main' ? 'block' : 'none';
    document.getElementById('screen-admin').style.display = name === 'admin' ? 'block' : 'none';
}

// --- Tela de Configuração ---
function loadConfig() {
    GetConfig().then(cfg => {
        document.getElementById('server_url').value = cfg.server_url || 'http://localhost:8888';
        document.getElementById('api_key').value = cfg.api_key || '';
        document.getElementById('client_id').value = cfg.client_id || '';
        document.getElementById('download_path').value = cfg.download_path || '';
        document.getElementById('site_mappings').value = cfg.site_mappings ? JSON.stringify(cfg.site_mappings, null, 2) : '';
    });
}

window.saveConfig = function () {
    const cfg = {
        server_url: document.getElementById('server_url').value.trim().replace(/\/$/, ''),
        api_key: document.getElementById('api_key').value.trim(),
        client_id: document.getElementById('client_id').value.trim(),
        download_path: document.getElementById('download_path').value.trim(),
        site_mappings: {}
    };
    
    const mappingsStr = document.getElementById('site_mappings').value.trim();
    if (mappingsStr) {
        try {
            cfg.site_mappings = JSON.parse(mappingsStr);
        } catch (e) {
            document.getElementById('status').innerText = 'JSON Inválido em Mapeamento de Sites.';
            return;
        }
    }

    if (!cfg.server_url || !cfg.api_key || !cfg.client_id) {
        document.getElementById('status').innerText = 'Preencha todos os campos obrigatórios.';
        return;
    }

    SaveConfig(cfg).then(() => {
        document.getElementById('status').innerText = '✅ Configurações salvas!';
        setTimeout(() => {
            document.getElementById('status').innerText = '';
            showScreen('main');
            startPolling();
        }, 1500);
    }).catch(err => {
        document.getElementById('status').innerText = '❌ Erro: ' + err;
    });
};

// Botão de engrenagem: abre configurações a partir da tela principal.
// Mostra o botão "Voltar" pois já há uma tela para retornar.
window.openSettings = function () {
    cameFromMain = true;
    document.getElementById('back-btn').style.display = 'inline-block';
    document.getElementById('status').innerText = '';
    loadConfig();
    showScreen('setup');
};

// Botão "Voltar": descarta alterações e retorna ao dashboard sem salvar.
window.cancelSetup = function () {
    cameFromMain = false;
    document.getElementById('back-btn').style.display = 'none';
    document.getElementById('status').innerText = '';
    showScreen('main');
};

// Sanitização contra Stored XSS
function escapeHtml(str) {
    if (!str) return '';
    return String(str)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#039;');
}

// --- Tela Principal (Dashboard) ---
function renderJobs(jobs) {
    const list = document.getElementById('jobs-list');
    if (!jobs || jobs.length === 0) {
        list.innerHTML = '<li class="empty">Nenhuma sincronização ativa</li>';
        return;
    }

    list.innerHTML = jobs.map(j => {
        const downloaded = j.downloaded || 0;
        const total = j.total_size || 0;
        const pct = total > 0 ? Math.min(100, Math.round((downloaded / total) * 100)) : 0;
        const statusIcon = j.status === 'Completed' ? '✅'
            : j.status === 'Error' ? '❌'
            : '⏳';
        const safeName = escapeHtml(j.file_name || 'arquivo');
        const safeStatus = escapeHtml(j.status || 'Pendente');
        return `
            <li class="job-item">
                <span class="job-name">${statusIcon} ${safeName}</span>
                <span class="job-status">${safeStatus} (${pct}%)</span>
                <div class="progress-bar">
                    <div class="progress-fill" style="width:${pct}%"></div>
                </div>
            </li>`;
    }).join('');
}

let pollingInterval = null;

function startPolling() {
    if (pollingInterval) return;
    pollingInterval = setInterval(() => {
        GetJobs().then(renderJobs);
    }, 2000);
    GetJobs().then(renderJobs); // busca imediata
}

// --- Init: decide qual tela mostrar ---
// No primeiro acesso, nenhuma configuração salva → tela de setup sem botão Voltar.
// Após configurado, vai direto para o dashboard.
IsConfigured().then(configured => {
    if (configured) {
        showScreen('main');
        startPolling();
    } else {
        cameFromMain = false;
        document.getElementById('back-btn').style.display = 'none';
        loadConfig();
        showScreen('setup');
    }
});

// --- Tela de Administração do Servidor ---
window.openAdmin = function () {
    showScreen('admin');
    window.loadServerConfig();
    window.loadAPIKeys();
    window.loadClients();
    window.loadServerFiles();
};

window.closeAdmin = function () {
    showScreen('main');
};

window.loadServerConfig = function () {
    GetServerConfig().then(cfg => {
        document.getElementById('server_storage_path').value = cfg.storage_path || '';
        document.getElementById('server_max_concurrent').value = cfg.max_concurrent_downloads || 8;
    }).catch(err => {
        const statusDiv = document.getElementById('admin-config-status');
        statusDiv.innerText = '❌ Erro ao carregar config: ' + err;
        statusDiv.style.color = '#ef4444';
    });
};

window.saveServerConfig = function () {
    const statusDiv = document.getElementById('admin-config-status');
    const cfg = {
        max_concurrent_downloads: parseInt(document.getElementById('server_max_concurrent').value) || 8,
    };
    
    UpdateServerConfig(cfg).then(newCfg => {
        statusDiv.innerText = '✅ Configuração do Servidor salva!';
        statusDiv.style.color = '#4ade80';
        setTimeout(() => statusDiv.innerText = '', 3000);
    }).catch(err => {
        statusDiv.innerText = '❌ Erro ao salvar config: ' + err;
        statusDiv.style.color = '#ef4444';
    });
};

window.cleanServer = function () {
    const statusDiv = document.getElementById('admin-clean-status');
    statusDiv.innerText = 'Limpando...';
    statusDiv.style.color = '#fff';
    CleanServer().then(msg => {
        statusDiv.innerText = '✅ ' + msg;
        statusDiv.style.color = '#4ade80';
        window.loadServerFiles(); // atualiza a lista após limpar
    }).catch(err => {
        statusDiv.innerText = '❌ Erro: ' + err;
        statusDiv.style.color = '#ef4444';
    });
};

window.loadServerFiles = function () {
    const list = document.getElementById('server-files-list');
    list.innerHTML = '<li>Carregando...</li>';
    ListServerFiles().then(files => {
        if (!files || files.length === 0) {
            list.innerHTML = '<li class="empty">Nenhum arquivo encontrado</li>';
            return;
        }
        
        list.innerHTML = files.map(f => {
            const sizeMB = (f.size / (1024 * 1024)).toFixed(2);
            let statusBadge = '';
            if (f.status === 'Completed') statusBadge = '✅';
            else if (f.status === 'Error') statusBadge = '❌';
            else if (f.status === 'Unknown') statusBadge = '👻';
            else statusBadge = '⏳';
            
            const safeName = escapeHtml(f.name || f.id);
            const safeId = escapeHtml(f.id);

            return `
                <li class="job-item" style="display:flex; justify-content:space-between; align-items:center;">
                    <div style="display:flex; align-items:center; gap: 10px;">
                        <input type="checkbox" class="file-checkbox" value="${safeId}">
                        <span>${statusBadge} ${safeName} <small>(${sizeMB} MB)</small></span>
                    </div>
                    <button class="icon-btn" onclick="deleteServerFile('${safeId}')" title="Excluir">🗑️</button>
                </li>
            `;
        }).join('');
    }).catch(err => {
        list.innerHTML = `<li class="empty" style="color:#ef4444;">Erro ao carregar arquivos: ${escapeHtml(err)}</li>`;
    });
};

window.deleteServerFile = function (fileId) {
    if (!confirm(`Tem certeza que deseja excluir este arquivo do servidor?`)) return;
    
    DeleteServerFile(fileId).then(() => {
        alert('✅ Arquivo excluído com sucesso!');
        window.loadServerFiles();
    }).catch(err => {
        alert('❌ Erro ao excluir: ' + err);
    });
};

function getSelectedFileIds() {
    const checkboxes = document.querySelectorAll('.file-checkbox:checked');
    return Array.from(checkboxes).map(cb => cb.value);
}

window.toggleSelectAll = function() {
    const checked = document.getElementById('select-all').checked;
    document.querySelectorAll('.file-checkbox').forEach(cb => {
        cb.checked = checked;
    });
};

window.deleteSelected = function() {
    const ids = getSelectedFileIds();
    if (ids.length === 0) return alert('Nenhum arquivo selecionado.');
    if (!confirm(`Excluir ${ids.length} arquivo(s) selecionado(s)?`)) return;

    DeleteBatchServerFiles(ids).then(() => {
        alert('✅ Arquivos excluídos com sucesso!');
        window.loadServerFiles();
        document.getElementById('select-all').checked = false;
    }).catch(err => alert('❌ Erro ao excluir em lote: ' + err));
};

window.syncSelected = function() {
    const ids = getSelectedFileIds();
    if (ids.length === 0) return alert('Nenhum arquivo selecionado.');
    if (!confirm(`Forçar o download de ${ids.length} arquivo(s) selecionado(s) para este cliente?`)) return;

    ForceSyncBatchServerFiles(ids).then(() => {
        alert('✅ Arquivos adicionados à fila de download do seu cliente!');
        document.getElementById('select-all').checked = false;
        // Volta pra main pra ver baixando
        window.closeAdmin();
    }).catch(err => alert('❌ Erro ao solicitar download em lote: ' + err));
};

window.loadAPIKeys = function() {
    const list = document.getElementById('api-keys-list');
    list.innerHTML = '<li>Carregando...</li>';
    ListAPIKeys().then(keys => {
        if (!keys || keys.length === 0) {
            list.innerHTML = '<li class="empty">Nenhuma chave encontrada</li>';
            return;
        }
        
        list.innerHTML = keys.map(k => {
            const safeName = escapeHtml(k.name || 'Sem nome');
            const safePrefix = escapeHtml(k.key_prefix || (k.key ? k.key.substring(0, 8) : ''));
            const keyId = escapeHtml(k.id || k.key || '');
            return `
                <li class="job-item" style="display:flex; justify-content:space-between; align-items:center;">
                    <div>
                        <strong>${safeName}</strong><br/>
                        <small style="font-family:monospace; color:#aaa;">Prefixo: ${safePrefix}...</small>
                    </div>
                    <button class="icon-btn" onclick="deleteAPIKey('${keyId}')" title="Excluir">🗑️</button>
                </li>
            `;
        }).join('');
    }).catch(err => {
        list.innerHTML = `<li class="empty" style="color:#ef4444;">Erro ao carregar chaves: ${escapeHtml(err)}</li>`;
    });
};

window.createAPIKey = function() {
    const nameInput = document.getElementById('new_api_key_name');
    const name = nameInput.value.trim();
    if (!name) return alert('Por favor, informe um nome para a chave.');
    
    const newKey = crypto.randomUUID();
    
    CreateAPIKey(name, newKey).then(() => {
        nameInput.value = '';
        alert('✅ Nova chave criada com sucesso!\nCopie a chave agora, pois você precisará configurá-ela no cliente.');
        window.loadAPIKeys();
    }).catch(err => {
        alert('❌ Erro ao criar chave: ' + err);
    });
};

window.deleteAPIKey = function(keyId) {
    if (!confirm('Tem certeza que deseja excluir esta chave de API? Qualquer cliente usando-a perderá acesso imediatamente.')) return;
    
    DeleteAPIKey(keyId).then(() => {
        alert('✅ Chave excluída com sucesso!');
        window.loadAPIKeys();
    }).catch(err => {
        alert('❌ Erro ao excluir chave: ' + err);
    });
};

window.loadClients = function() {
    const list = document.getElementById('clients-list');
    list.innerHTML = '<li>Carregando...</li>';
    ListClients().then(clients => {
        if (!clients || clients.length === 0) {
            list.innerHTML = '<li class="empty">Nenhum cliente conectado recentemente</li>';
            return;
        }
        
        list.innerHTML = clients.map(c => {
            const date = new Date(c.last_seen).toLocaleString();
            const safeId = escapeHtml(c.id);
            const safeIp = escapeHtml(c.ip);
            return `
                <li class="job-item">
                    <strong>${safeId}</strong><br/>
                    <small style="color:#aaa;">Último acesso: ${date} - IP: ${safeIp}</small>
                </li>
            `;
        }).join('');
    }).catch(err => {
        list.innerHTML = `<li class="empty" style="color:#ef4444;">Erro ao carregar clientes: ${escapeHtml(err)}</li>`;
    });
};
