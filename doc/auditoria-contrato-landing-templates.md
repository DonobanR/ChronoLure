# Auditoría de contrato y estructura de landing templates (basada en código y commits)

Este documento se basa **exclusivamente** en el código y el historial disponible del repositorio. Cuando una afirmación no puede comprobarse en el repositorio, se marca como **no observable en el repositorio**.

---

## 1) Cómo funcionaban los landing templates ANTES (según el código)

**Limitación histórica:** el primer commit (`5ce8db9`) ya incluye la infraestructura calendar/ICS y los templates actuales. No hay versiones anteriores de templates en el historial. Por lo tanto, los patrones “anteriores” solo pueden inferirse de los **templates actuales** que ya existían en el commit inicial.

### Patrones reales observados en templates

#### a) Interceptar submit con JavaScript
**Archivo:** `templates/calendar_landing.html`
```html
<form id="credentialsForm" method="POST">
  ...
</form>

<script>
  document.getElementById('credentialsForm').addEventListener('submit', function(e) {
    e.preventDefault();
    const formData = new FormData(this);
    fetch(window.location.href, {
      method: 'POST',
      body: formData
    })
    .then(response => response.json())
    .then(data => {
      if (data.redirect) {
        window.location.href = data.redirect;
      } else {
        window.location.href = '/';
      }
    })
    .catch(() => {
      window.location.href = '/';
    });
  });
</script>
```
**Rol del template según el código:**
- Intercepta el submit (`preventDefault`).
- Envía el POST manualmente con `fetch`.
- Lee JSON de respuesta y decide la redirección en cliente.

#### b) Uso de XMLHttpRequest y redirección forzada
**Archivo:** `templates/teams_login.html`
```html
<form id="loginForm" method="POST">
  ...
</form>

<script>
  document.getElementById('loginForm').addEventListener('submit', function(e) {
    e.preventDefault();
    var email = document.getElementById('email').value;
    var password = document.getElementById('password').value;

    var xhr = new XMLHttpRequest();
    xhr.open('POST', window.location.href, true);
    xhr.setRequestHeader('Content-Type', 'application/x-www-form-urlencoded');

    setTimeout(function() {
      window.location.href = 'https://teams.microsoft.com';
    }, 1500);

    xhr.send('email=' + encodeURIComponent(email) + '&password=' + encodeURIComponent(password));
  });
</script>
```
**Rol del template según el código:**
- Intercepta submit (`preventDefault`).
- Envía credenciales por XHR.
- Fuerza redirección en cliente **sin esperar respuesta** del backend.

### Conclusión observable del “antes”
- Los templates incluidos en el repo **ya** implementan control de submit y redirección del lado del cliente.
- **No observable en el repositorio**: versiones previas sin JS o con comportamiento distinto.

---

## 2) Qué cambios de código invalidan esos patrones

### Cambios relacionados con Calendar Invite / EventMeetingURL
**Commit relevante:** `2b47046`.

**Cambio concreto:**
- `controllers/calendar.go` en `handleCalendarPhishPOST` cambia la fuente de redirección:
  - Antes (según diff): `redirectURL := c.Page.RedirectURL`.
  - Después: `redirectURL := c.EventMeetingURL`, con fallback a `c.Page.RedirectURL`.

**Efecto observable sobre expectativas de templates:**
- Si un template **ignora** el JSON de redirección y fuerza un URL fijo (ej. `teams_login.html`), el valor de `EventMeetingURL` **no tiene efecto** para ese template.
  - Esto no rompe el POST, pero **rompe la expectativa** de que la redirección final provenga del backend.

**Cambios que invaliden patrones como “leer JSON” o “usar fetch/XHR”:**
- **No observable en el repositorio**. No hay commits que eliminen respuestas JSON ni que deshabiliten POST vía XHR/fetch en `/calendar`.

---

## 3) Cambio de contrato backend ↔ landing template

### Contrato observable en backend (no especulativo)

#### a) Flujo **calendar** (`/calendar`)
**Backend:** `controllers/calendar.go:handleCalendarPhishPOST`
- Responde **JSON**:
  - `{"redirect": "<url>", "message": "success"}`.
- La redirección se **decide en backend** (`EventMeetingURL` o `Page.RedirectURL`) y el cliente debe aplicarla.

**Implicación observable:**
- El template debe interpretar JSON si desea usar la redirección definida por backend.

#### b) Flujo **estándar** (`PhishHandler`)
**Backend:** `controllers/phish.go:renderPhishResponse`
- Si es POST y `Page.RedirectURL` está definido, el servidor hace `http.Redirect` (respuesta 302). No hay JSON.

**Implicación observable:**
- En el flujo estándar, el backend **no entrega JSON**; responde con redirección HTTP.

### Qué responsabilidades pasan a backend / cliente (según código)
- **Backend decide la URL final** en `/calendar` (JSON con `redirect`). `controllers/calendar.go`.
- **Cliente ejecuta la redirección** en templates calendar (si consume JSON). `templates/calendar_landing.html`.
- **No observable en el repositorio**: eliminación de respuestas JSON en `/calendar` o migración del control de redirect completamente al servidor en ese flujo.

---

## 4) Ejemplos de templates incompatibles (con código real)

### a) Template que intercepta submit y fuerza redirect fijo
**Archivo:** `templates/teams_login.html`
```html
setTimeout(function() {
  window.location.href = 'https://teams.microsoft.com';
}, 1500);
```
**Por qué es incompatible con el contrato backend (observable):**
- El backend devuelve `redirect` en JSON (`handleCalendarPhishPOST`), pero este template **no lo lee**.
- Resultado: `EventMeetingURL` (introducido como prioridad en commit `2b47046`) **no se aplica**.

### b) Template que no interpreta JSON de `/calendar`
**Archivo base observable:** `templates/calendar_landing.html` **sí** interpreta JSON.

**No observable en el repositorio:**
- No existe en el repo un template que haga POST a `/calendar` sin consumir el JSON. Por lo tanto, no hay ejemplo real adicional.

---

## 5) Estructura mínima válida de un landing template AHORA (basado en backend)

### Qué espera realmente el backend al recibir POST

#### En `/calendar` (`handleCalendarPhishPOST`)
- Se llama a `r.ParseForm()`.
- Se lee explícitamente `email := r.FormValue("email")` para registrar detalles en `calendar_events`.
- Todo el formulario se almacena vía `rs.HandleFormSubmit(details)` (payload completo).

**Conclusión observable:**
- El formulario puede contener cualquier campo; el backend **solo lee `email` explícitamente**.
- No hay validación de campos en backend más allá de `ParseForm`.

### Estructura mínima observable derivada de templates actuales
**Archivo:** `templates/calendar_landing.html`
```html
<form id="credentialsForm" method="POST">
  <input type="email" id="email" name="email" required>
  <input type="password" id="password" name="password" required>
  <button type="submit">Unirse Ahora</button>
</form>
```
**Qué debe contener (según backend):**
- Método `POST`.
- Campo `email` si se quiere que `calendar_events.details` incluya el email.

**Qué lógica debe desaparecer (solo si se quiere respetar redirect backend):**
- Redirecciones fijas en cliente que ignoren el JSON del backend. (Ejemplo real: `templates/teams_login.html`.)

---

## 6) Evidencia concreta (tabla)

| Archivo / commit | Comportamiento observado | Impacto en landing templates |
| --- | --- | --- |
| `templates/calendar_landing.html` | Intercepta submit, usa `fetch`, lee JSON y redirige | Template depende del JSON `redirect` de `/calendar` |
| `templates/teams_login.html` | Intercepta submit con XHR y redirige fijo | Ignora `redirect` del backend en `/calendar` |
| `controllers/calendar.go` | POST devuelve JSON con `redirect` (y ahora prioriza `EventMeetingURL`) | El template debe consumir JSON para respetar la redirección configurada |
| Commit `2b47046` | Cambia la fuente de `redirect` a `EventMeetingURL` | Templates que fuerzan redirect fijo no reflejan este cambio |
| `controllers/phish.go` | POST estándar redirige con `http.Redirect` (sin JSON) | Templates en flujo estándar no deben esperar JSON |

---

## 7) Conclusión estrictamente basada en código

**Patrones de templates obsoletos (en términos de contrato observable):**
- Templates calendar que **ignoran** el JSON de `/calendar` hacen que `EventMeetingURL` no se aplique. (Ejemplo real: `templates/teams_login.html`.)

**Patrones que siguen siendo válidos:**
- Interceptar submit con `fetch` y redirigir usando `data.redirect`. (Ejemplo real: `templates/calendar_landing.html`.)
- En flujo estándar (no calendar), permitir que el backend redirija vía HTTP 302. (`controllers/phish.go`.)

**No observable en el repositorio:**
- Cambios que eliminen JSON en `/calendar`.
- Modificaciones históricas en templates previas al commit inicial.
