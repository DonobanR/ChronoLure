# Migración de landing pages tras el cambio de arquitectura en GoPhish

Este documento explica **cómo funcionaban las landing pages antes** y **cómo deben funcionar ahora**, con ejemplos concretos de HTML correcto e incorrecto. Está dirigido a personas sin conocimiento previo de GoPhish que necesitan **modificar landing pages existentes** sin romper campañas.

Reglas de lectura:
- Cada concepto viene con un ejemplo realista.
- Si algo “rompe”, se muestra cómo rompe.
- No se describen cambios abstractos: se explica el **cómo exacto**.

---

## 1. Qué era una landing page ANTES (con ejemplo real)

### Rol antiguo (explicación + ejemplo)
Antes, una landing page era **presentación + control de flujo**. Además de mostrar contenido, **decidía a dónde se redirigía el usuario** después del submit. Esa decisión estaba embebida en el HTML (atributo `action`) o en JavaScript.

### ❌ Ejemplo típico de landing page antigua (incorrecta ahora)

```html
<!DOCTYPE html>
<html>
  <head>
    <title>Verifica tu cuenta</title>
    <script>
      // Redirección forzada después del submit
      function redirectAfterSubmit() {
        window.location = "https://accounts.example.com/login";
        return false;
      }
    </script>
  </head>
  <body>
    <h1>Acceso seguro</h1>
    <form action="https://accounts.example.com/login" method="POST" onsubmit="return redirectAfterSubmit()">
      <input type="text" name="usuario" />
      <input type="password" name="clave" />
      <button type="submit">Continuar</button>
    </form>
  </body>
</html>
```

#### Línea por línea: qué hace cada parte
- `<script>...redirectAfterSubmit()</script>`: define una función que **fuerza** la navegación a una URL externa.
- `<form action="https://accounts.example.com/login" ...>`: el `action` **envía los datos fuera de GoPhish**.
- `onsubmit="return redirectAfterSubmit()"`: al enviar, **ni siquiera espera la respuesta de GoPhish**.

#### En qué punto se decide la redirección
La redirección se decide **antes del procesamiento backend** porque el HTML define el destino y el JS lo ejecuta inmediatamente.

#### Por qué GoPhish pierde el control del flujo
GoPhish no puede decidir nada porque:
- El navegador **no envía el POST a GoPhish** (va a `https://accounts.example.com/login`).
- Aunque llegara un POST, el `onsubmit` **corta el flujo** y fuerza la navegación externa.

---

## 2. Qué problema EXACTO causa ese HTML

### Qué request NO llega a GoPhish
El POST del formulario **no llega** a GoPhish. Va directo al `action` externo.

**Resultado técnico:**
- GoPhish no registra el evento de submit.
- No se guarda usuario/clave ni timestamp.

### Qué datos no se registran
- Campos enviados (`usuario`, `clave`).
- Métricas de conversión asociadas al submit.
- Cualquier dato de tracking vinculado al formulario.

### Qué ve el usuario
El usuario ve una redirección inmediata a `https://accounts.example.com/login`, sin importar si GoPhish procesó o no.

### Qué ve el operador de campaña
- Ve que la landing fue abierta (si el GET sí pasó por GoPhish).
- **No ve submit registrado**.
- Los reportes muestran aperturas sin conversiones, aunque el usuario haya “enviado”.

---

## 3. Cambio de contrato: quién manda ahora

### Explicación + ejemplo concreto
Ahora **la landing page NO puede decidir redirección**. Solo el formulario (configurado en GoPhish) puede hacerlo.

**Frase de contrato:**
> “Si la landing page intenta definir un destino, el sistema se comporta como si el submit nunca hubiese pasado por GoPhish.”

**Ejemplo concreto:**
- Si mantienes `action="https://accounts.example.com/login"`, el POST **sale del sistema**.
- Si mantienes `window.location` en `onsubmit`, el navegador **no espera la respuesta de GoPhish**.

**Consecuencia:** el backend pierde control y la campaña queda sin datos de submit.

---

## 4. Nueva landing page CORRECTA (con ejemplo)

### ✅ Ejemplo de landing page adaptada correctamente

```html
<!DOCTYPE html>
<html>
  <head>
    <title>Verifica tu cuenta</title>
  </head>
  <body>
    <h1>Acceso seguro</h1>
    <form method="POST">
      <input type="text" name="usuario" />
      <input type="password" name="clave" />
      <button type="submit">Continuar</button>
    </form>
  </body>
</html>
```

### Qué se eliminó y por qué
- **Se eliminó `action` externo**: evita que el POST salga de GoPhish.
- **Se eliminó `onsubmit`**: evita redirecciones prematuras.
- **No hay scripts de control**: el backend decide el destino.

**Resultado técnico:** el POST llega a GoPhish, se registra y luego se redirige desde el sistema.

---

## 5. DÓNDE se define ahora la redirección (con ejemplo conceptual)

### Explicación + ejemplo
La redirección ahora se define **en el formulario configurado en GoPhish**, no en el HTML.

**Ejemplo conceptual de formulario:**

```
Formulario:
  - Campos: usuario, password
  - Redirect URL: https://example.com/login
```

### Cuándo se ejecuta esa redirección
1. El navegador envía el POST a GoPhish.
2. GoPhish registra el submit y guarda los datos.
3. El sistema responde con redirección a la URL configurada en el formulario.

**Clave:** la redirección ocurre **después del procesamiento**, no antes.

---

## 6. Migración paso a paso (guía de cirujano)

### Checklist accionable
- **Busca esto en tu HTML → elimínalo:**
  - `action="http..."` hacia dominios externos.
  - `onsubmit="..."` con redirecciones o `return false`.
  - Scripts con `window.location`, `document.location`, `location.href` tras submit.

- **Si ves este patrón → está mal:**
  - Formulario con `action` que no apunta a GoPhish.
  - JavaScript que reescribe el `action` dinámicamente.
  - Dos formularios con destinos distintos en la misma landing.

- **Si necesitas cambiar la redirección → NO toques la landing:**
  - Cambia la URL de redirección **en el formulario de GoPhish**.
  - La landing solo debe contener HTML pasivo.

---

## 7. Casos reales de error

### Caso 1: Landing no adaptada
**Qué hace el usuario:** completa el formulario y hace submit.

**Qué hace GoPhish:** recibe el GET inicial, **no recibe el POST**.

**Qué sale mal:**
- El operador ve aperturas sin submits.
- El usuario es redirigido fuera del sistema sin registro.

**Ejemplo que rompe:**
```html
<form action="https://accounts.example.com/login" method="POST">
  <input type="text" name="usuario" />
  <input type="password" name="clave" />
  <button type="submit">Continuar</button>
</form>
```

### Caso 2: Landing parcialmente adaptada
**Qué hace el usuario:** envía el formulario, pero un script fuerza redirección temprana.

**Qué hace GoPhish:** recibe el POST, pero el navegador navega fuera antes de completar el flujo.

**Qué sale mal:**
- El submit puede registrarse, pero la redirección es inconsistente.
- El usuario no sigue el destino definido por el formulario.

**Ejemplo que rompe:**
```html
<form method="POST" onsubmit="window.location='https://accounts.example.com/login'; return false;">
  <input type="text" name="usuario" />
  <input type="password" name="clave" />
  <button type="submit">Continuar</button>
</form>
```

### Caso 3: Landing correcta
**Qué hace el usuario:** envía el formulario.

**Qué hace GoPhish:** procesa el submit, guarda datos, decide redirección.

**Qué sale bien:**
- El operador ve aperturas y submits consistentes.
- La redirección es la configurada en el formulario, no en el HTML.

**Ejemplo correcto:**
```html
<form method="POST">
  <input type="text" name="usuario" />
  <input type="password" name="clave" />
  <button type="submit">Continuar</button>
</form>
```
