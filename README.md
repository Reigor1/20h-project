golang todoapp

# Заметки:
- в features/transport/http
- в Makefile используется абсолютный путь в место относительных
- ErrUnknown = errors.New("unknown error") для того чтобы библиотека не протекала 13 27
- могут быть проблемы из за некорректной 
- limit и offset их проверка в на уровне сервиса, возможно стоит вынести в доменную область
- накатить и использовать линтер
- обсудить проблему дублирования кода type `TaskModel` tasks and statistics feature