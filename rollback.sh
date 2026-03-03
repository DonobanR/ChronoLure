#!/bin/bash
# Script de Rollback para ChronoLure
# Ejecutar si algo sale mal después del deployment

set -e

echo "=========================================="
echo "   ROLLBACK - ChronoLure"
echo "=========================================="
echo ""

# Colores
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Directorio base
BASE_DIR="/home/dono/ChronoLure"
cd "$BASE_DIR"

echo -e "${YELLOW}Opciones de rollback:${NC}"
echo "1) Restaurar código original (git restore)"
echo "2) Restaurar desde backup manual (backups/)"
echo "3) Cancelar"
echo ""
read -p "Selecciona una opción [1-3]: " option

case $option in
    1)
        echo -e "${YELLOW}Restaurando código original desde git...${NC}"
        git restore controllers/calendar.go
        echo -e "${GREEN}✓ Código restaurado desde git${NC}"
        ;;
    2)
        echo -e "${YELLOW}Backups disponibles:${NC}"
        ls -lht backups/ | head -10
        echo ""
        read -p "Ingresa el nombre del directorio de backup (ej: 20260106_131022): " backup_dir
        
        if [ -f "backups/$backup_dir/calendar.go.backup" ]; then
            cp "backups/$backup_dir/calendar.go.backup" controllers/calendar.go
            echo -e "${GREEN}✓ Archivo restaurado desde backup${NC}"
        else
            echo -e "${RED}✗ Backup no encontrado${NC}"
            exit 1
        fi
        ;;
    3)
        echo "Rollback cancelado"
        exit 0
        ;;
    *)
        echo -e "${RED}Opción inválida${NC}"
        exit 1
        ;;
esac

echo ""
echo -e "${YELLOW}¿Recompilar el proyecto? [y/N]:${NC}"
read -p "" recompile

if [[ $recompile =~ ^[Yy]$ ]]; then
    echo -e "${YELLOW}Recompilando...${NC}"
    go build
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓ Compilación exitosa${NC}"
        echo ""
        echo -e "${YELLOW}Para reiniciar el servicio, ejecuta:${NC}"
        echo "  ./gophish"
        echo "  o"
        echo "  sudo systemctl restart gophish"
    else
        echo -e "${RED}✗ Error en la compilación${NC}"
        exit 1
    fi
else
    echo "Rollback completo. Recuerda recompilar cuando estés listo."
fi

echo ""
echo -e "${GREEN}=========================================="
echo "   Rollback completado"
echo "==========================================${NC}"
