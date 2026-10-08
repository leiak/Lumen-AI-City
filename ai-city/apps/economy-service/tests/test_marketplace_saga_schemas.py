import pytest
from economy_service.schemas.marketplace_saga import SagaTemplateCreate
from economy_service.services.template_validator import YamlInvalidError, validate_saga_yaml
from pydantic import ValidationError

VALID_YAML = "saga:\n  name: welcome\n  steps:\n    - task: hello\n"


def test_accepts_valid_saga_yaml():
    validate_saga_yaml(VALID_YAML)


def test_rejects_invalid_saga_yaml():
    with pytest.raises(YamlInvalidError, match="YAML parse error"):
        validate_saga_yaml("name: 'foo")


@pytest.mark.parametrize(
    "yaml_text",
    [
        "!!python/object/apply:os.system ['echo pwned']",
        "!!python/name:os.system ['echo pwned']",
        "!!python/module:os.system",
        "!!python/object/new:builtins.eval ['return 1']",
    ],
)
def test_rejects_python_object_tags(yaml_text):
    with pytest.raises(YamlInvalidError, match="Forbidden YAML tag"):
        validate_saga_yaml(yaml_text)


def test_saga_template_create_valid():
    template = SagaTemplateCreate(
        name="Welcome Saga",
        yaml_content=VALID_YAML,
        npc_deps=["npc_wang_boss_001"],
        semantic_version="1.0.0",
    )

    assert template.name == "Welcome Saga"
    assert template.semantic_version == "1.0.0"


def test_saga_template_create_rejects_bad_semver():
    with pytest.raises(ValidationError, match="string_pattern_mismatch"):
        SagaTemplateCreate(
            name="Welcome Saga",
            yaml_content=VALID_YAML,
            semantic_version="1.0",
        )
