# Introducción del canal Calendar Invite (ICS) en una plataforma web multicanal

Este documento describe, de forma cronológica y técnica, la incorporación de un nuevo canal de interacción basado en invitaciones de calendario (Calendar Invite / ICS) dentro de una plataforma web existente. El objetivo es mostrar **cómo cambia el flujo completo**, qué componentes se ven afectados y cómo implementar correctamente el nuevo canal sin perder trazabilidad.

---

## Estado inicial (antes del cambio)

### Cómo funcionaba el sistema originalmente
El sistema operaba con un flujo principal basado en email:
1. Se generaba una campaña con emails.
2. El usuario recibía el correo.
3. El usuario hacía clic en el enlace.
4. El navegador abría una landing page.
5. El backend registraba eventos (apertura, clic, submit).

### Canales existentes
- **Email**: canal primario de distribución.
- **Landing page**: punto de interacción y captura.

### Componentes que dependían del flujo original
- **Backend/API**: esperaba que la interacción iniciara en un clic de email.
- **Tracking**: se basaba en métricas de email (open/click/submit).
- **Modelos de datos**: asociaban eventos a un email y una landing.
- **Reporting**: asumía que todo tráfico pasaba por el navegador.
- **Estados de campaña**: dependían del ciclo “email → click → landing”.

---

## Introducción del Calendar Invite (el cambio)

### En qué punto se añade el calendario
El calendario se introduce **después de la generación del mensaje**, pero **antes de la interacción en navegador**. En vez de solo enviar un email con link, el sistema adjunta o genera una invitación ICS.

### Qué nuevo evento/interacción se introduce
Se introduce una **interacción fuera del navegador**:
- Recepción de un evento en el cliente de calendario.
- Apertura, aceptación o rechazo del evento **sin pasar por la landing**.

### Diferencia frente al email tradicional
- **Email**: el clic es la principal señal de interacción.
- **Calendar Invite**: la señal puede ocurrir en el cliente de calendario, sin clic ni navegador.
- **Resultado**: el sistema debe registrar eventos en más de un canal para no perder trazabilidad.

---

## Cronología técnica completa del flujo

### 1) Generación del evento de calendario
- El backend genera un archivo ICS con:
  - Identificador único del evento.
  - UID correlacionable con la campaña.
  - URL o referencia de tracking embebida en la descripción o en el enlace.
- Este evento se asocia a un destinatario específico.

### 2) Distribución al usuario
- El evento se entrega:
  - Como adjunto en email.
  - O como link directo a un archivo .ics.

### 3) Qué ocurre cuando el usuario:

#### a) Solo recibe el evento
- El cliente de correo descarga el adjunto.
- **Se registra apertura de email**, pero **no hay interacción de calendario** aún.
- Señal recibida por: sistema de tracking de email.

#### b) Lo abre
- El usuario abre el evento en el cliente de calendario.
- Se genera un evento de “visualización” (si el cliente lo reporta).
- Señal recibida por: backend de tracking de calendario (si existe callback/URL embebida).

#### c) Lo acepta
- El cliente de calendario puede enviar una respuesta (RSVP).
- El sistema puede registrar “evento aceptado” vía tracking embebido o buzón de respuestas.
- Señal recibida por: backend de campaña, modelo de eventos de calendario.

#### d) Lo ignora
- No hay clic ni submit.
- El sistema solo registra la entrega (email) o la apertura de email, si ocurrió.
- Señal recibida por: tracking de email.

---

## Impacto del cambio en otros componentes

### Backend / API
- Debe soportar **nuevos tipos de eventos** (open/accept/decline de calendario).
- Debe correlacionar el UID del evento con el destinatario.
- Debe exponer endpoints para tracking de calendario.

### Sistema de tracking
- Ya no puede depender solo de clicks.
- Debe integrar señales de calendarios y mapearlas a campañas.

### Base de datos / modelo de eventos
- Se introducen nuevos campos:
  - UID de evento.
  - Estado de aceptación.
  - Timestamps de interacción fuera de navegador.

### Landing page
- Deja de ser el único punto de interacción.
- No todos los usuarios llegarán a la landing.

### Métricas y reporting
- Se añade un nuevo funnel:
  - Recibido → Abierto en calendario → Aceptado → (opcional) clic → landing.
- Se deben mostrar interacciones sin clic.

### Automatizaciones o estados de campaña
- Las campañas deben considerar estados de calendario:
  - “Aceptado” puede ser un indicador de éxito.
  - “Ignorado” no implica fallo si no hay clic.

---

## Errores comunes al implementar este cambio

### 1) Enfocarse solo en la landing
Resultado:
- Se pierden interacciones que ocurren en el cliente de calendario.
- Los reportes muestran falsos negativos.

### 2) Asumir que todo pasa por el navegador
Resultado:
- El backend no registra eventos de aceptación/rechazo.
- El funnel queda incompleto.

### 3) Duplicar eventos o perder trazabilidad
Resultado:
- Si el UID no es único por destinatario, se mezclan eventos.
- Si no hay correlación con campaña, se pierde la relación con métricas.

---

## Cómo debe implementarse correctamente ahora

### Fuente de verdad
La fuente de verdad debe ser el **modelo de eventos del backend**, no el navegador ni el cliente de calendario.

### Correlación entre calendario, email y landing
- Cada envío debe tener:
  - ID de campaña.
  - ID de destinatario.
  - UID de evento ICS.
- Los eventos deben vincularse a ese triplete.

### Buenas prácticas de diseño del flujo
- Registrar **todas las interacciones** posibles:
  - Apertura de email.
  - Apertura de evento.
  - Aceptación/rechazo.
  - Clic en link (si ocurre).
  - Submit en landing.
- No asumir que un canal reemplaza al otro: el calendario es **aditivo**.

### Qué eventos deben registrarse y cuáles no
- **Registrar**: apertura de email, apertura de evento, aceptación/rechazo, clic, submit.
- **No registrar**: eventos duplicados del mismo UID en el mismo estado (evitar ruido).

---

## Resumen ejecutivo del cambio

### Qué cambió realmente
Se añadió un canal de interacción **fuera del navegador** que introduce señales nuevas (apertura/aceptación de calendario) y rompe el supuesto original de que todo flujo pasa por la landing.

### Por qué el impacto va más allá del landing
El nuevo canal afecta tracking, modelo de datos, métricas y estados de campaña. La landing se convierte en un canal opcional, no obligatorio.

### Qué mentalidad debe tener el equipo
El equipo debe pensar en **flujos multicanal**: la interacción del usuario puede ocurrir en correo, calendario o navegador, y todos esos eventos deben correlacionarse en backend para mantener trazabilidad completa.
