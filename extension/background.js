chrome.runtime.onInstalled.addListener(() => {
    chrome.contextMenus.create({
        id: "machdown-download",
        title: "Baixar com MachDown",
        contexts: ["link", "video", "audio", "image"]
    });
});

chrome.contextMenus.onClicked.addListener(async (info, tab) => {
    if (info.menuItemId === "machdown-download") {
        const urlToDownload = info.linkUrl || info.srcUrl;
        forwardToMachdown(urlToDownload);
    }
});

// Intercept native browser downloads
chrome.downloads.onCreated.addListener((downloadItem) => {
    // Ignore internal extension downloads or object URLs to avoid loops
    if (downloadItem.url.startsWith("blob:") || 
        downloadItem.url.startsWith("data:") || 
        downloadItem.url.startsWith("chrome-extension:")) {
        return;
    }

    chrome.storage.sync.get(['intercept_enabled'], (config) => {
        if (config.intercept_enabled === false) {
            console.log("Interceptação desativada. Permitindo download nativo:", downloadItem.url);
            return;
        }

        console.log("Interceptando download nativo:", downloadItem.url);

        // Forward to our MachDown server immediately
        forwardToMachdown(downloadItem.url);

        // Cancel the native browser download
        chrome.downloads.cancel(downloadItem.id).catch(err => {
            console.error("Falha ao cancelar download nativo:", err);
        });
    });
});

function forwardToMachdown(urlToDownload) {
    chrome.storage.sync.get(['server_url', 'api_key', 'client_id'], async (config) => {
        if (!config.server_url || !config.api_key) {
            chrome.notifications.create({
                type: "basic",
                iconUrl: "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=",
                title: "MachDown: Erro de Configuração",
                message: "Clique no ícone da extensão e configure a URL do servidor e a API Key."
            });
            return;
        }

        let cookieStr = "";
        try {
            const cookies = await chrome.cookies.getAll({ url: urlToDownload });
            cookieStr = cookies.map(c => c.name + "=" + c.value).join("; ");
        } catch(e) {
            console.error("Failed to get cookies:", e);
        }

        try {
            const response = await fetch(`${config.server_url}/api/downloads`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'X-API-Key': config.api_key
                },
                body: JSON.stringify({
                    url: urlToDownload,
                    target_clients: [config.client_id || ''],
                    cookies: cookieStr,
                    user_agent: navigator.userAgent
                })
            });

            if (response.ok) {
                chrome.notifications.create({
                    type: "basic",
                    iconUrl: "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=",
                    title: "MachDown",
                    message: "Download capturado e enviado pro Servidor!"
                });
            } else {
                const data = await response.json();
                throw new Error(data.error || 'Erro desconhecido');
            }
        } catch (err) {
            chrome.notifications.create({
                type: "basic",
                iconUrl: "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=",
                title: "MachDown: Falha ao enviar",
                message: err.toString()
            });
        }
    });
}
