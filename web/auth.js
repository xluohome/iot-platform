const API_BASE = 'http://localhost:8080/api/v1';

function getAuthHeader() {
    const token = localStorage.getItem('access_token');
    return token ? { 'Authorization': `Bearer ${token}` } : {};
}

async function checkAuth() {
    const accessToken = localStorage.getItem('access_token');
    const userStr = localStorage.getItem('user');

    if (!accessToken || !userStr) {
        redirectToLogin();
        return null;
    }

    try {
        const resp = await fetch(`${API_BASE}/auth/me`, {
            headers: getAuthHeader()
        });

        if (resp.ok) {
            const user = await resp.json();
            localStorage.setItem('user', JSON.stringify(user));
            return user;
        }
    } catch (err) {
        console.error('Auth check error:', err);
    }

    const refreshed = await refreshAccessToken();
    if (!refreshed) {
        redirectToLogin();
        return null;
    }

    return JSON.parse(localStorage.getItem('user'));
}

async function refreshAccessToken() {
    const refreshToken = localStorage.getItem('refresh_token');
    if (!refreshToken) return false;

    try {
        const resp = await fetch(`${API_BASE}/auth/refresh`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ refresh_token: refreshToken })
        });

        if (!resp.ok) {
            logout();
            return false;
        }

        const data = await resp.json();
        localStorage.setItem('access_token', data.access_token);

        const userResp = await fetch(`${API_BASE}/auth/me`, {
            headers: { 'Authorization': `Bearer ${data.access_token}` }
        });

        if (userResp.ok) {
            const user = await userResp.json();
            localStorage.setItem('user', JSON.stringify(user));
            return true;
        }
    } catch (err) {
        console.error('Token refresh error:', err);
    }

    return false;
}

function logout() {
    const refreshToken = localStorage.getItem('refresh_token');

    fetch(`${API_BASE}/auth/logout`, {
        method: 'POST',
        headers: {
            ...getAuthHeader(),
            'X-Refresh-Token': refreshToken || ''
        }
    }).catch(err => console.error('Logout error:', err));

    localStorage.removeItem('access_token');
    localStorage.removeItem('refresh_token');
    localStorage.removeItem('user');

    redirectToLogin();
}

function redirectToLogin() {
    window.location.href = 'login.html';
}

function getCurrentUser() {
    const userStr = localStorage.getItem('user');
    return userStr ? JSON.parse(userStr) : null;
}

async function authFetch(url, options = {}) {
    const headers = {
        ...getAuthHeader(),
        'Content-Type': 'application/json',
        ...options.headers
    };

    let response = await fetch(url, { ...options, headers });

    if (response.status === 401) {
        const refreshed = await refreshAccessToken();
        if (refreshed) {
            headers['Authorization'] = `Bearer ${localStorage.getItem('access_token')}`;
            response = await fetch(url, { ...options, headers });
        } else {
            redirectToLogin();
            return null;
        }
    }

    return response;
}

window.auth = {
    checkAuth,
    refreshAccessToken,
    logout,
    getAuthHeader,
    getCurrentUser,
    authFetch,
    redirectToLogin
};