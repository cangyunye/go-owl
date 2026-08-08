export function renderLogin(render, navigate) {
  render(`
    <div class="login-page">
      <div class="login-card">
        <h2>OWL Console</h2>
        <div class="form-group">
          <label for="username">Username</label>
          <input type="text" id="username" placeholder="Enter username" autocomplete="username">
        </div>
        <div class="form-group">
          <label for="password">Password</label>
          <input type="password" id="password" placeholder="Enter password" autocomplete="current-password">
        </div>
        <button id="login-btn">Sign In</button>
        <p class="error-msg" id="login-error"></p>
      </div>
    </div>
  `, () => {
    const btn = document.getElementById('login-btn');
    const username = document.getElementById('username');
    const password = document.getElementById('password');
    const error = document.getElementById('login-error');

    async function doLogin() {
      if (!username.value || !password.value) {
        error.textContent = 'Please enter username and password';
        return;
      }
      btn.disabled = true;
      btn.textContent = 'Signing in...';
      try {
        const res = await fetch('/api/v1/login', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ username: username.value, password: password.value }),
        });
        if (!res.ok) {
          const err = await res.json();
          error.textContent = err.message || 'Invalid credentials';
          btn.disabled = false;
          btn.textContent = 'Sign In';
          return;
        }
        const data = await res.json();
        localStorage.setItem('token', data.token);
        localStorage.setItem('user', JSON.stringify(data.user));
        navigate('/');
      } catch (e) {
        error.textContent = 'Connection error';
        btn.disabled = false;
        btn.textContent = 'Sign In';
      }
    }

    btn.addEventListener('click', doLogin);
    password.addEventListener('keydown', (e) => { if (e.key === 'Enter') doLogin(); });
  });
}
