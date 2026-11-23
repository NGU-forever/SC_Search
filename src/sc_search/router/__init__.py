"""Router: validate primary+slots and expand INTENT_TOOLS into a DAG plan."""

from sc_search.router.dag import expand_dag
from sc_search.router.schema import RoutePlan, validate_route

__all__ = ["RoutePlan", "expand_dag", "validate_route"]
